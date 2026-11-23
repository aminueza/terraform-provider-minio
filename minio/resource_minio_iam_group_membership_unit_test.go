package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func groupMembershipStubAdmin(t *testing.T, handler http.HandlerFunc) *madmin.AdminClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}
	return client
}

func TestIAMGroupMembershipDeleteSkipsEmptyMembers(t *testing.T) {
	calls := 0
	client := groupMembershipStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	})

	res := resourceMinioIAMGroupMembership()
	d := res.TestResourceData()
	if err := d.Set("name", "team-membership"); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("group", "test-group"); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("users", []string{}); err != nil {
		t.Fatal(err)
	}
	d.SetId("team-membership")

	diags := res.DeleteContext(context.Background(), d, &S3MinioClient{S3Admin: client})
	if diags.HasError() {
		t.Fatalf("expected delete with empty users to succeed, got %v", diags)
	}
	if calls != 0 {
		t.Fatalf("expected 0 API calls when users is empty, got %d", calls)
	}
	if d.Id() != "" {
		t.Fatalf("expected state ID to be cleared, got %q", d.Id())
	}
}

func TestIAMGroupMembershipDeleteLeavesOtherMembers(t *testing.T) {
	var mu sync.Mutex
	members := []string{"a", "b"}
	groupDeleted := false

	client := groupMembershipStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/update-group-members") {
			http.NotFound(w, r)
			return
		}
		var req madmin.GroupAddRemove
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()

		if req.IsRemove && len(req.Members) == 0 {
			groupDeleted = true
			w.WriteHeader(http.StatusOK)
			return
		}
		if req.IsRemove {
			filtered := members[:0]
			for _, existing := range members {
				if !slices.Contains(req.Members, existing) {
					filtered = append(filtered, existing)
				}
			}
			members = filtered
		}
		w.WriteHeader(http.StatusOK)
	})

	res := resourceMinioIAMGroupMembership()
	d := res.TestResourceData()
	if err := d.Set("name", "team-membership"); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("group", "test-group"); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("users", []string{"a"}); err != nil {
		t.Fatal(err)
	}
	d.SetId("team-membership")

	diags := res.DeleteContext(context.Background(), d, &S3MinioClient{S3Admin: client})
	if diags.HasError() {
		t.Fatalf("expected delete to succeed, got %v", diags)
	}
	if groupDeleted {
		t.Fatal("delete unexpectedly requested group deletion")
	}
	if !slices.Equal(members, []string{"b"}) {
		t.Fatalf("expected remaining members [b], got %v", members)
	}
}

func TestIAMGroupMembershipGroupLookup500ReturnsErrorWithoutPanic(t *testing.T) {
	client := groupMembershipStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/group") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			if err := json.NewEncoder(w).Encode(map[string]string{
				"Code":    "InternalError",
				"Message": "simulated 500 error",
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	cfg := &S3MinioIAMGroupMembershipConfig{
		MinioAdmin:    client,
		MinioIAMName:  "team-membership",
		MinioIAMGroup: "test-group",
		MinioIAMUsers: []string{"alice"},
	}

	if err := userToADD(context.Background(), cfg, []string{"bob"}); err == nil {
		t.Fatal("userToADD: expected error when group lookup returns 500, got nil")
	}
	if err := userToRemove(context.Background(), cfg, []string{"alice"}); err == nil {
		t.Fatal("userToRemove: expected error when group lookup returns 500, got nil")
	}
}

func TestIAMGroupMembershipDeleteToleratesMissingTargets(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    string
		message string
	}{
		{
			name:    "missing user counts as already removed",
			code:    "XMinioAdminNoSuchUser",
			message: "The specified user does not exist.",
		},
		{
			name:    "missing group counts as already destroyed",
			code:    "XMinioAdminNoSuchGroup",
			message: "The specified group does not exist.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := groupMembershipStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				if err := json.NewEncoder(w).Encode(map[string]string{
					"Code":    tc.code,
					"Message": tc.message,
				}); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
			})

			res := resourceMinioIAMGroupMembership()
			d := res.TestResourceData()
			if err := d.Set("name", "team-membership"); err != nil {
				t.Fatal(err)
			}
			if err := d.Set("group", "test-group"); err != nil {
				t.Fatal(err)
			}
			if err := d.Set("users", []string{"alice"}); err != nil {
				t.Fatal(err)
			}
			d.SetId("team-membership")

			diags := res.DeleteContext(context.Background(), d, &S3MinioClient{S3Admin: client})
			if diags.HasError() {
				t.Fatalf("expected delete to tolerate %s, got %v", tc.code, diags)
			}
			if d.Id() != "" {
				t.Fatalf("expected state ID to be cleared, got %q", d.Id())
			}
		})
	}

	// The helpers must match only their own code, so a real failure is never
	// swallowed by the tolerated-missing paths.
	if isGroupMembershipGroupMissing(madmin.ErrorResponse{Code: "XMinioAdminNoSuchKey"}) ||
		isGroupMembershipUserMissing(madmin.ErrorResponse{Code: "XMinioAdminNoSuchKey"}) {
		t.Fatal("missing-target helpers matched an unrelated error code")
	}
	if !isGroupMembershipUserMissing(madmin.ErrorResponse{Code: "XMinioAdminNoSuchUser"}) {
		t.Fatal("isGroupMembershipUserMissing did not match XMinioAdminNoSuchUser")
	}
	if !isGroupMembershipGroupMissing(madmin.ErrorResponse{Code: "XMinioAdminNoSuchGroup"}) {
		t.Fatal("isGroupMembershipGroupMissing did not match XMinioAdminNoSuchGroup")
	}
}

func TestIAMGroupMembershipHelpersStrictChecks(t *testing.T) {
	// Attribute check must reject substring group names and extra/missing members.
	if err := checkMinioGroupMembershipAttributes(&madmin.GroupDesc{
		Name:    "team-extra",
		Members: []string{"alice"},
	}, "team", []string{"alice"}); err == nil {
		t.Fatal("expected substring group name mismatch to fail")
	}
	if err := checkMinioGroupMembershipAttributes(&madmin.GroupDesc{
		Name:    "team",
		Members: []string{"alice", "extra"},
	}, "team", []string{"alice"}); err == nil {
		t.Fatal("expected extra member in group to fail")
	}
	if err := checkMinioGroupMembershipAttributes(&madmin.GroupDesc{
		Name:    "team",
		Members: []string{"bob", "alice"},
	}, "team", []string{"alice", "bob"}); err != nil {
		t.Fatalf("expected unordered equal members to pass, got %v", err)
	}

	// Destroy check must fail on 500/200 and only succeed on missing group.
	for _, tc := range []struct {
		name    string
		code    string
		message string
		status  int
		wantErr bool
	}{
		{
			name:    "still exists",
			status:  http.StatusOK,
			wantErr: true,
		},
		{
			name:    "500 error does not count as destroyed",
			status:  http.StatusInternalServerError,
			code:    "InternalError",
			message: "internal failure",
			wantErr: true,
		},
		{
			name:    "missing group counts as destroyed",
			status:  http.StatusNotFound,
			code:    "XMinioAdminNoSuchGroup",
			message: "The specified group does not exist.",
			wantErr: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := groupMembershipStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if tc.status == http.StatusOK {
					w.WriteHeader(http.StatusOK)
					if encErr := json.NewEncoder(w).Encode(madmin.GroupDesc{Name: "team"}); encErr != nil {
						http.Error(w, encErr.Error(), http.StatusInternalServerError)
					}
					return
				}
				w.WriteHeader(tc.status)
				if encErr := json.NewEncoder(w).Encode(map[string]string{
					"Code":    tc.code,
					"Message": tc.message,
				}); encErr != nil {
					http.Error(w, encErr.Error(), http.StatusInternalServerError)
				}
			})

			err := checkMinioGroupMembershipDestroyed(context.Background(), client, "team")
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestIAMGroupMembershipSchemaAndCreateSetsGroupID(t *testing.T) {
	res := resourceMinioIAMGroupMembership()
	if res.Schema["name"].ForceNew {
		t.Fatal("expected schema attribute \"name\" to have ForceNew=false")
	}

	client := groupMembershipStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/update-group-members"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/group"):
			if got := r.URL.Query().Get("group"); got != "test-group" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(madmin.GroupDesc{
				Name:    "test-group",
				Members: []string{"alice"},
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		default:
			http.NotFound(w, r)
		}
	})

	d := res.TestResourceData()
	if err := d.Set("name", "team-membership"); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("group", "test-group"); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("users", []string{"alice"}); err != nil {
		t.Fatal(err)
	}

	diags := res.CreateContext(context.Background(), d, &S3MinioClient{S3Admin: client})
	if diags.HasError() {
		t.Fatalf("CreateContext returned error: %v", diags)
	}
	if d.Id() != "test-group" {
		t.Fatalf("expected CreateContext to set ID to group name %q, got %q", "test-group", d.Id())
	}
	if got := d.Get("group").(string); got != "test-group" {
		t.Fatalf("expected group=%q, got %q", "test-group", got)
	}
}

func TestIAMGroupMembershipImport(t *testing.T) {
	ctx := context.Background()

	t.Run("imports existing group into state", func(t *testing.T) {
		client := groupMembershipStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/group") || r.URL.Query().Get("group") != "test-group" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"Code":    "XMinioAdminNoSuchGroup",
					"Message": "The specified group does not exist.",
				})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(madmin.GroupDesc{
				Name:    "test-group",
				Members: []string{"alice", "bob"},
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		})

		res := resourceMinioIAMGroupMembership()
		d := res.TestResourceData()
		d.SetId("test-group")

		imported, err := res.Importer.StateContext(ctx, d, &S3MinioClient{S3Admin: client})
		if err != nil {
			t.Fatalf("Importer.StateContext returned error: %v", err)
		}
		if len(imported) != 1 {
			t.Fatalf("expected 1 imported ResourceData, got %d", len(imported))
		}

		diags := res.ReadContext(ctx, imported[0], &S3MinioClient{S3Admin: client})
		if diags.HasError() {
			t.Fatalf("ReadContext after import returned error: %v", diags)
		}
		if imported[0].Id() != "test-group" {
			t.Fatalf("expected ID %q after import, got %q", "test-group", imported[0].Id())
		}
		if got := imported[0].Get("group").(string); got != "test-group" {
			t.Fatalf("expected group=%q after import, got %q", "test-group", got)
		}
		if got := imported[0].Get("name").(string); got != "" {
			t.Fatalf("expected name to remain empty after import, got %q", got)
		}

		rawUsers := imported[0].Get("users").(*schema.Set).List()
		gotUsers := make([]string, len(rawUsers))
		for i, u := range rawUsers {
			gotUsers[i] = u.(string)
		}
		sort.Strings(gotUsers)
		if !slices.Equal(gotUsers, []string{"alice", "bob"}) {
			t.Fatalf("expected users [alice bob], got %v", gotUsers)
		}
	})

	t.Run("imports group when Name field omitted in response", func(t *testing.T) {
		client := groupMembershipStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/group") || r.URL.Query().Get("group") != "test-group" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(madmin.GroupDesc{
				Members: []string{"alice"},
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		})

		res := resourceMinioIAMGroupMembership()
		d := res.TestResourceData()
		d.SetId("test-group")

		imported, err := res.Importer.StateContext(ctx, d, &S3MinioClient{S3Admin: client})
		if err != nil {
			t.Fatalf("Importer.StateContext returned error: %v", err)
		}
		diags := res.ReadContext(ctx, imported[0], &S3MinioClient{S3Admin: client})
		if diags.HasError() {
			t.Fatalf("ReadContext returned error: %v", diags)
		}
		if imported[0].Id() != "test-group" {
			t.Fatalf("expected ID %q, got %q", "test-group", imported[0].Id())
		}
		if got := imported[0].Get("group").(string); got != "test-group" {
			t.Fatalf("expected group=%q, got %q", "test-group", got)
		}
		rawUsers := imported[0].Get("users").(*schema.Set).List()
		if len(rawUsers) != 1 || rawUsers[0].(string) != "alice" {
			t.Fatalf("expected users [alice], got %v", rawUsers)
		}
	})

	for _, tc := range []struct {
		name    string
		message string
	}{
		{
			name:    "missing group clears state on standard message",
			message: "The specified group does not exist.",
		},
		{
			name:    "missing group clears state on code only message",
			message: "group absent",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := groupMembershipStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"Code":    "XMinioAdminNoSuchGroup",
					"Message": tc.message,
				})
			})

			res := resourceMinioIAMGroupMembership()
			d := res.TestResourceData()
			d.SetId("missing-group")

			imported, err := res.Importer.StateContext(ctx, d, &S3MinioClient{S3Admin: client})
			if err != nil {
				t.Fatalf("Importer.StateContext returned unexpected error: %v", err)
			}
			diags := res.ReadContext(ctx, imported[0], &S3MinioClient{S3Admin: client})
			if diags.HasError() {
				t.Fatalf("expected missing group read to clear state without error, got %v", diags)
			}
			if imported[0].Id() != "" {
				t.Fatalf("expected state ID to be cleared so Terraform reports non-existent object, got %q", imported[0].Id())
			}
		})
	}

	t.Run("empty import ID fails before API calls", func(t *testing.T) {
		calls := 0
		client := groupMembershipStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusInternalServerError)
		})

		res := resourceMinioIAMGroupMembership()
		d := res.TestResourceData()
		d.SetId("")

		if _, err := res.Importer.StateContext(ctx, d, &S3MinioClient{S3Admin: client}); err == nil {
			t.Fatal("expected Importer.StateContext with empty ID to fail, got nil")
		}
		if calls != 0 {
			t.Fatalf("expected 0 API calls when import ID is empty, got %d", calls)
		}
	})
}

func TestIAMGroupMembershipUpgradeReadMigratesIDAndPlansInPlaceUpdate(t *testing.T) {
	ctx := context.Background()
	updateCalls := 0
	client := groupMembershipStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/update-group-members") {
			updateCalls++
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/group") && r.URL.Query().Get("group") == "test-group" {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(madmin.GroupDesc{
				Name:    "test-group",
				Members: []string{"alice"},
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		http.NotFound(w, r)
	})
	meta := &S3MinioClient{S3Admin: client}

	res := resourceMinioIAMGroupMembership()
	d := res.TestResourceData()
	if err := d.Set("name", "legacy-label"); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("group", "test-group"); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("users", []string{"alice"}); err != nil {
		t.Fatal(err)
	}
	d.SetId("legacy-label")

	if diags := res.ReadContext(ctx, d, meta); diags.HasError() {
		t.Fatalf("ReadContext on legacy state failed: %v", diags)
	}
	if d.Id() != "test-group" {
		t.Fatalf("expected legacy state ID to migrate to group name %q, got %q", "test-group", d.Id())
	}
	if got := d.Get("name").(string); got != "legacy-label" {
		t.Fatalf("expected name %q to be preserved, got %q", "legacy-label", got)
	}

	importedState := &terraform.InstanceState{
		ID: "test-group",
		Attributes: map[string]string{
			"id":      "test-group",
			"group":   "test-group",
			"users.#": "1",
			fmt.Sprintf("users.%d", schema.HashString("alice")): "alice",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]interface{}{
		"name":  "team-membership",
		"group": "test-group",
		"users": []interface{}{"alice"},
	})
	diff, err := res.Diff(ctx, importedState, cfg, meta)
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if diff == nil || diff.Empty() {
		t.Fatal("expected non-empty in-place diff for name after import")
	}
	if diff.RequiresNew() {
		t.Fatalf("expected in-place update after import without replacement, got RequiresNew diff: %#v", diff)
	}
	if attrDiff, ok := diff.Attributes["name"]; !ok || attrDiff.RequiresNew {
		t.Fatalf("expected name diff without RequiresNew, got %#v", diff.Attributes["name"])
	}

	// Applying an in-place name update must not touch group membership on MinIO.
	if diags := res.UpdateContext(ctx, d, meta); diags.HasError() {
		t.Fatalf("UpdateContext returned error: %v", diags)
	}
	if updateCalls != 0 {
		t.Fatalf("expected 0 UpdateGroupMembers API calls on name-only update, got %d", updateCalls)
	}
}
