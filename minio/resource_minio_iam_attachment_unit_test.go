package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func newIAMAttachmentStubClient(t *testing.T, handler http.HandlerFunc) *S3MinioClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}
	return &S3MinioClient{S3Admin: admin}
}

func TestIAMAttachmentIDsCreateReadImportAndMigrate(t *testing.T) {
	ctx := context.Background()

	t.Run("user policy attachment create read migrate missing import", func(t *testing.T) {
		attached := false
		userGone := false
		meta := newIAMAttachmentStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/user-info"):
				if userGone {
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, `{"Code":"XMinioAdminNoSuchUser","Message":"The user specified does not exist"}`)
				} else if attached {
					_, _ = fmt.Fprint(w, `{"status":"enabled","policyName":"readwrite","memberOf":[]}`)
				} else {
					_, _ = fmt.Fprint(w, `{"status":"enabled","policyName":"","memberOf":[]}`)
				}
			case strings.HasSuffix(r.URL.Path, "/idp/builtin/policy/attach"):
				attached = true
				w.WriteHeader(http.StatusNoContent)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		})

		res := resourceMinioIAMUserPolicyAttachment()
		d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
			"user_name":   "alice",
			"policy_name": "readwrite",
		})
		if diags := res.CreateContext(ctx, d, meta); diags.HasError() {
			t.Fatalf("create failed: %v", diags)
		}
		if d.Id() != "alice/readwrite" {
			t.Fatalf("create id = %q, want %q", d.Id(), "alice/readwrite")
		}

		// Legacy pre-migration state migrates ID in place with an empty plan.
		legacyState := &terraform.InstanceState{
			ID: "alice-20261123010026647900000001",
			Attributes: map[string]string{
				"id":          "alice-20261123010026647900000001",
				"user_name":   "alice",
				"policy_name": "readwrite",
			},
		}
		refreshed, diags := res.RefreshWithoutUpgrade(ctx, legacyState, meta)
		if diags.HasError() {
			t.Fatalf("refresh failed: %v", diags)
		}
		if refreshed == nil || refreshed.ID != "alice/readwrite" {
			t.Fatalf("refreshed id = %v, want %q", refreshed, "alice/readwrite")
		}
		cfg := terraform.NewResourceConfigRaw(map[string]interface{}{
			"user_name":   "alice",
			"policy_name": "readwrite",
		})
		diff, err := res.Diff(ctx, refreshed, cfg, meta)
		if err != nil {
			t.Fatalf("diff failed: %v", err)
		}
		if !diff.Empty() {
			t.Fatalf("expected empty plan after ID migration, got diff: %#v", diff)
		}

		// Missing attachment removes resource from state instead of re-setting ID.
		attached = false
		gone, diags := res.RefreshWithoutUpgrade(ctx, refreshed, meta)
		if diags.HasError() {
			t.Fatalf("refresh when missing failed: %v", diags)
		}
		if gone != nil && gone.ID != "" {
			t.Fatalf("expected resource removal when detached, got state %q", gone.ID)
		}

		// Deleted user also clears state: minioReadUserPolicies maps
		// XMinioAdminNoSuchUser to "no policies" and Read drops the resource.
		userGone = true
		goneUser, diags := res.RefreshWithoutUpgrade(ctx, refreshed, meta)
		if diags.HasError() {
			t.Fatalf("refresh when user deleted failed: %v", diags)
		}
		if goneUser != nil && goneUser.ID != "" {
			t.Fatalf("expected resource removal when user deleted, got state %q", goneUser.ID)
		}
		userGone = false

		imp := res.Data(nil)
		imp.SetId("alice/readwrite")
		imported, err := res.Importer.StateContext(ctx, imp, meta)
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		if len(imported) != 1 || imported[0].Id() != "alice/readwrite" {
			t.Fatalf("imported = %#v, want id %q", imported, "alice/readwrite")
		}
		if imported[0].Get("user_name") != "alice" || imported[0].Get("policy_name") != "readwrite" {
			t.Fatalf("imported attrs = (%v, %v), want (alice, readwrite)", imported[0].Get("user_name"), imported[0].Get("policy_name"))
		}

		for _, badID := range []string{"alice", "alice/", "/readwrite", "readwrite/cn=alice,ou=users,dc=example,dc=com"} {
			bad := res.Data(nil)
			bad.SetId(badID)
			if _, err := res.Importer.StateContext(ctx, bad, meta); err == nil {
				t.Fatalf("expected import error for malformed ID %q, got nil", badID)
			}
		}
	})

	t.Run("group policy attachment create read migrate missing import", func(t *testing.T) {
		attached := false
		groupExists := true
		meta := newIAMAttachmentStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/group"):
				if !groupExists {
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, `{"Code":"XMinioAdminNoSuchGroup","Message":"The specified group does not exist."}`)
					return
				}
				if attached {
					_, _ = fmt.Fprint(w, `{"name":"devs","status":"enabled","members":[],"policy":"readonly"}`)
				} else {
					_, _ = fmt.Fprint(w, `{"name":"devs","status":"enabled","members":[],"policy":""}`)
				}
			case strings.HasSuffix(r.URL.Path, "/idp/builtin/policy/attach"):
				attached = true
				w.WriteHeader(http.StatusNoContent)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		})

		res := resourceMinioIAMGroupPolicyAttachment()
		d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
			"group_name":  "devs",
			"policy_name": "readonly",
		})
		if diags := res.CreateContext(ctx, d, meta); diags.HasError() {
			t.Fatalf("create failed: %v", diags)
		}
		if d.Id() != "devs/readonly" {
			t.Fatalf("create id = %q, want %q", d.Id(), "devs/readonly")
		}

		legacyState := &terraform.InstanceState{
			ID: "devs-20261123010026647900000001",
			Attributes: map[string]string{
				"id":          "devs-20261123010026647900000001",
				"group_name":  "devs",
				"policy_name": "readonly",
			},
		}
		refreshed, diags := res.RefreshWithoutUpgrade(ctx, legacyState, meta)
		if diags.HasError() {
			t.Fatalf("refresh failed: %v", diags)
		}
		if refreshed == nil || refreshed.ID != "devs/readonly" {
			t.Fatalf("refreshed id = %v, want %q", refreshed, "devs/readonly")
		}
		cfg := terraform.NewResourceConfigRaw(map[string]interface{}{
			"group_name":  "devs",
			"policy_name": "readonly",
		})
		diff, err := res.Diff(ctx, refreshed, cfg, meta)
		if err != nil {
			t.Fatalf("diff failed: %v", err)
		}
		if !diff.Empty() {
			t.Fatalf("expected empty plan after ID migration, got diff: %#v", diff)
		}

		// Detached policy clears ID cleanly.
		attached = false
		gone, diags := res.RefreshWithoutUpgrade(ctx, refreshed, meta)
		if diags.HasError() || (gone != nil && gone.ID != "") {
			t.Fatalf("expected removal when detached, got state=%v diags=%v", gone, diags)
		}
		attached = true
		groupExists = false
		goneGroup, diags := res.RefreshWithoutUpgrade(ctx, refreshed, meta)
		if diags.HasError() || (goneGroup != nil && goneGroup.ID != "") {
			t.Fatalf("expected removal when group missing, got state=%v diags=%v", goneGroup, diags)
		}

		imp := res.Data(nil)
		imp.SetId("devs/readonly")
		imported, err := res.Importer.StateContext(ctx, imp, meta)
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		if len(imported) != 1 || imported[0].Id() != "devs/readonly" {
			t.Fatalf("imported = %#v, want id %q", imported, "devs/readonly")
		}
		if imported[0].Get("group_name") != "devs" || imported[0].Get("policy_name") != "readonly" {
			t.Fatalf("imported attrs = (%v, %v), want (devs, readonly)", imported[0].Get("group_name"), imported[0].Get("policy_name"))
		}

		for _, badID := range []string{"devs", "devs/", "/readonly", "readonly/cn=devs,ou=groups,dc=example,dc=com"} {
			bad := res.Data(nil)
			bad.SetId(badID)
			if _, err := res.Importer.StateContext(ctx, bad, meta); err == nil {
				t.Fatalf("expected import error for malformed ID %q, got nil", badID)
			}
		}
	})

	t.Run("group user attachment create read migrate missing import", func(t *testing.T) {
		memberPresent := false
		meta := newIAMAttachmentStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/update-group-members"):
				memberPresent = true
				w.WriteHeader(http.StatusOK)
			case strings.HasSuffix(r.URL.Path, "/group"):
				if memberPresent {
					_, _ = fmt.Fprint(w, `{"name":"devs","status":"enabled","members":["alice"],"policy":""}`)
				} else {
					_, _ = fmt.Fprint(w, `{"name":"devs","status":"enabled","members":[],"policy":""}`)
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		})

		res := resourceMinioIAMGroupUserAttachment()
		d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
			"group_name": "devs",
			"user_name":  "alice",
		})
		if diags := res.CreateContext(ctx, d, meta); diags.HasError() {
			t.Fatalf("create failed: %v", diags)
		}
		if d.Id() != "devs/alice" {
			t.Fatalf("create id = %q, want %q", d.Id(), "devs/alice")
		}

		legacyState := &terraform.InstanceState{
			ID: "devs/alice20261123010647105400000001",
			Attributes: map[string]string{
				"id":         "devs/alice20261123010647105400000001",
				"group_name": "devs",
				"user_name":  "alice",
			},
		}
		refreshed, diags := res.RefreshWithoutUpgrade(ctx, legacyState, meta)
		if diags.HasError() {
			t.Fatalf("refresh failed: %v", diags)
		}
		if refreshed == nil || refreshed.ID != "devs/alice" {
			t.Fatalf("refreshed id = %v, want %q", refreshed, "devs/alice")
		}
		cfg := terraform.NewResourceConfigRaw(map[string]interface{}{
			"group_name": "devs",
			"user_name":  "alice",
		})
		diff, err := res.Diff(ctx, refreshed, cfg, meta)
		if err != nil {
			t.Fatalf("diff failed: %v", err)
		}
		if !diff.Empty() {
			t.Fatalf("expected empty plan after ID migration, got diff: %#v", diff)
		}

		memberPresent = false
		gone, diags := res.RefreshWithoutUpgrade(ctx, refreshed, meta)
		if diags.HasError() {
			t.Fatalf("refresh when missing failed: %v", diags)
		}
		if gone != nil && gone.ID != "" {
			t.Fatalf("expected resource removal when user removed from group, got state %q", gone.ID)
		}

		imp := res.Data(nil)
		imp.SetId("devs/alice")
		imported, err := res.Importer.StateContext(ctx, imp, meta)
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		if len(imported) != 1 || imported[0].Id() != "devs/alice" {
			t.Fatalf("imported = %#v, want id %q", imported, "devs/alice")
		}
		if imported[0].Get("group_name") != "devs" || imported[0].Get("user_name") != "alice" {
			t.Fatalf("imported attrs = (%v, %v), want (devs, alice)", imported[0].Get("group_name"), imported[0].Get("user_name"))
		}

		for _, badID := range []string{"devs", "devs/", "/alice"} {
			bad := res.Data(nil)
			bad.SetId(badID)
			if _, err := res.Importer.StateContext(ctx, bad, meta); err == nil {
				t.Fatalf("expected import error for malformed ID %q, got nil", badID)
			}
		}
	})
}
