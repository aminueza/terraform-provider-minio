package minio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func iamMissingTargetAdminStub(t *testing.T, handler http.HandlerFunc) *madmin.AdminClient {
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

func writeMadminStubError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"Code":    code,
		"Message": message,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeMadminStubJSON(w http.ResponseWriter, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func TestIAMGroupUserAttachmentMissingTarget(t *testing.T) {
	res := resourceMinioIAMGroupUserAttachment()

	readCases := []struct {
		name     string
		code     string
		message  string
		status   int
		isNew    bool
		wantErr  bool
		wantGone bool
	}{
		{
			name:     "missing group clears id",
			status:   http.StatusNotFound,
			code:     "XMinioAdminNoSuchGroup",
			message:  "group does not exist",
			isNew:    false,
			wantErr:  false,
			wantGone: true,
		},
		{
			name:     "missing group after create still errors",
			status:   http.StatusNotFound,
			code:     "XMinioAdminNoSuchGroup",
			message:  "group does not exist",
			isNew:    true,
			wantErr:  true,
			wantGone: false,
		},
		{
			name:     "access denied still errors",
			status:   http.StatusForbidden,
			code:     "AccessDenied",
			message:  "Access Denied.",
			isNew:    false,
			wantErr:  true,
			wantGone: false,
		},
	}

	for _, tc := range readCases {
		t.Run("read/"+tc.name, func(t *testing.T) {
			client := iamMissingTargetAdminStub(t, func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/group") {
					t.Errorf("unexpected path %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				writeMadminStubError(w, tc.status, tc.code, tc.message)
			})

			d := res.TestResourceData()
			if err := d.Set("group_name", "test-group"); err != nil {
				t.Fatal(err)
			}
			if err := d.Set("user_name", "test-user"); err != nil {
				t.Fatal(err)
			}
			d.SetId("test-group:test-user")
			if tc.isNew {
				d.MarkNewResource()
			}

			diags := res.ReadContext(context.Background(), d, &S3MinioClient{S3Admin: client})
			if diags.HasError() != tc.wantErr {
				t.Fatalf("wantErr=%v, got diags=%v", tc.wantErr, diags)
			}
			if gotGone := d.Id() == ""; gotGone != tc.wantGone {
				t.Fatalf("wantGone=%v, got id=%q", tc.wantGone, d.Id())
			}
		})
	}

	deleteCases := []struct {
		name    string
		code    string
		message string
		status  int
		wantErr bool
	}{
		{
			name:    "missing group succeeds",
			status:  http.StatusNotFound,
			code:    "XMinioAdminNoSuchGroup",
			message: "group does not exist",
			wantErr: false,
		},
		{
			name:    "missing user succeeds",
			status:  http.StatusNotFound,
			code:    "XMinioAdminNoSuchUser",
			message: "user does not exist",
			wantErr: false,
		},
		{
			name:    "access denied errors",
			status:  http.StatusForbidden,
			code:    "AccessDenied",
			message: "Access Denied.",
			wantErr: true,
		},
	}

	for _, tc := range deleteCases {
		t.Run("delete/"+tc.name, func(t *testing.T) {
			client := iamMissingTargetAdminStub(t, func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/update-group-members") {
					t.Errorf("unexpected path %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				writeMadminStubError(w, tc.status, tc.code, tc.message)
			})

			d := res.TestResourceData()
			if err := d.Set("group_name", "test-group"); err != nil {
				t.Fatal(err)
			}
			if err := d.Set("user_name", "test-user"); err != nil {
				t.Fatal(err)
			}
			d.SetId("test-group:test-user")

			diags := res.DeleteContext(context.Background(), d, &S3MinioClient{S3Admin: client})
			if diags.HasError() != tc.wantErr {
				t.Fatalf("wantErr=%v, got diags=%v", tc.wantErr, diags)
			}
		})
	}
}

func TestIAMGroupPolicyAttachmentMissingTarget(t *testing.T) {
	res := resourceMinioIAMGroupPolicyAttachment()

	readCases := []struct {
		name     string
		code     string
		message  string
		status   int
		isNew    bool
		wantErr  bool
		wantGone bool
	}{
		{
			name:     "missing group clears id",
			status:   http.StatusNotFound,
			code:     "XMinioAdminNoSuchGroup",
			message:  "group does not exist",
			isNew:    false,
			wantErr:  false,
			wantGone: true,
		},
		{
			name:     "missing group after create still errors",
			status:   http.StatusNotFound,
			code:     "XMinioAdminNoSuchGroup",
			message:  "group does not exist",
			isNew:    true,
			wantErr:  true,
			wantGone: false,
		},
		{
			name:     "access denied still errors",
			status:   http.StatusForbidden,
			code:     "AccessDenied",
			message:  "Access Denied.",
			isNew:    false,
			wantErr:  true,
			wantGone: false,
		},
	}

	for _, tc := range readCases {
		t.Run("read/"+tc.name, func(t *testing.T) {
			client := iamMissingTargetAdminStub(t, func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/group") {
					t.Errorf("unexpected path %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				writeMadminStubError(w, tc.status, tc.code, tc.message)
			})

			d := res.TestResourceData()
			if err := d.Set("group_name", "test-group"); err != nil {
				t.Fatal(err)
			}
			if err := d.Set("policy_name", "test-policy"); err != nil {
				t.Fatal(err)
			}
			d.SetId("test-group:test-policy")
			if tc.isNew {
				d.MarkNewResource()
			}

			diags := res.ReadContext(context.Background(), d, &S3MinioClient{S3Admin: client})
			if diags.HasError() != tc.wantErr {
				t.Fatalf("wantErr=%v, got diags=%v", tc.wantErr, diags)
			}
			if gotGone := d.Id() == ""; gotGone != tc.wantGone {
				t.Fatalf("wantGone=%v, got id=%q", tc.wantGone, d.Id())
			}
		})
	}

	deleteCases := []struct {
		handler http.HandlerFunc
		name    string
		wantErr bool
	}{
		{
			name: "missing group on read succeeds",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/group") {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				writeMadminStubError(w, http.StatusNotFound, "XMinioAdminNoSuchGroup", "group does not exist")
			},
			wantErr: false,
		},
		{
			name: "access denied on read errors",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeMadminStubError(w, http.StatusForbidden, "AccessDenied", "Access Denied.")
			},
			wantErr: true,
		},
		{
			name: "missing group on detach succeeds",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/group"):
					writeMadminStubJSON(w, madmin.GroupDesc{
						Name:   "test-group",
						Status: "enabled",
						Policy: "test-policy",
					})
				case strings.HasSuffix(r.URL.Path, "/idp/builtin/policy/detach"):
					writeMadminStubError(w, http.StatusNotFound, "XMinioAdminNoSuchGroup", "group does not exist")
				default:
					http.NotFound(w, r)
				}
			},
			wantErr: false,
		},
		{
			name: "access denied on detach errors",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/group"):
					writeMadminStubJSON(w, madmin.GroupDesc{
						Name:   "test-group",
						Status: "enabled",
						Policy: "test-policy",
					})
				case strings.HasSuffix(r.URL.Path, "/idp/builtin/policy/detach"):
					writeMadminStubError(w, http.StatusForbidden, "AccessDenied", "Access Denied.")
				default:
					http.NotFound(w, r)
				}
			},
			wantErr: true,
		},
	}

	for _, tc := range deleteCases {
		t.Run("delete/"+tc.name, func(t *testing.T) {
			client := iamMissingTargetAdminStub(t, tc.handler)

			d := res.TestResourceData()
			if err := d.Set("group_name", "test-group"); err != nil {
				t.Fatal(err)
			}
			if err := d.Set("policy_name", "test-policy"); err != nil {
				t.Fatal(err)
			}
			d.SetId("test-group:test-policy")

			diags := res.DeleteContext(context.Background(), d, &S3MinioClient{S3Admin: client})
			if diags.HasError() != tc.wantErr {
				t.Fatalf("wantErr=%v, got diags=%v", tc.wantErr, diags)
			}
		})
	}
}

func TestIAMUserGroupMembershipMissingTarget(t *testing.T) {
	res := resourceMinioIAMUserGroupMembership()

	readCases := []struct {
		name     string
		code     string
		message  string
		status   int
		isNew    bool
		wantErr  bool
		wantGone bool
	}{
		{
			name:     "missing user clears id",
			status:   http.StatusNotFound,
			code:     "XMinioAdminNoSuchUser",
			message:  "user does not exist",
			isNew:    false,
			wantErr:  false,
			wantGone: true,
		},
		{
			name:     "missing user after create still errors",
			status:   http.StatusNotFound,
			code:     "XMinioAdminNoSuchUser",
			message:  "user does not exist",
			isNew:    true,
			wantErr:  true,
			wantGone: false,
		},
		{
			name:     "access denied still errors",
			status:   http.StatusForbidden,
			code:     "AccessDenied",
			message:  "Access Denied.",
			isNew:    false,
			wantErr:  true,
			wantGone: false,
		},
	}

	for _, tc := range readCases {
		t.Run("read/"+tc.name, func(t *testing.T) {
			client := iamMissingTargetAdminStub(t, func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/user-info") {
					t.Errorf("unexpected path %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				writeMadminStubError(w, tc.status, tc.code, tc.message)
			})

			d := res.TestResourceData()
			if err := d.Set("user", "test-user"); err != nil {
				t.Fatal(err)
			}
			if err := d.Set("groups", []string{"group-a", "group-b"}); err != nil {
				t.Fatal(err)
			}
			d.SetId("test-user")
			if tc.isNew {
				d.MarkNewResource()
			}

			diags := res.ReadContext(context.Background(), d, &S3MinioClient{S3Admin: client})
			if diags.HasError() != tc.wantErr {
				t.Fatalf("wantErr=%v, got diags=%v", tc.wantErr, diags)
			}
			if gotGone := d.Id() == ""; gotGone != tc.wantGone {
				t.Fatalf("wantGone=%v, got id=%q", tc.wantGone, d.Id())
			}
		})
	}

	t.Run("delete/missing user succeeds", func(t *testing.T) {
		client := iamMissingTargetAdminStub(t, func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/update-group-members") {
				t.Errorf("unexpected path %s %s", r.Method, r.URL.Path)
				http.NotFound(w, r)
				return
			}
			writeMadminStubError(w, http.StatusNotFound, "XMinioAdminNoSuchUser", "user does not exist")
		})

		d := res.TestResourceData()
		if err := d.Set("user", "test-user"); err != nil {
			t.Fatal(err)
		}
		if err := d.Set("groups", []string{"group-a", "group-b"}); err != nil {
			t.Fatal(err)
		}
		d.SetId("test-user")

		diags := res.DeleteContext(context.Background(), d, &S3MinioClient{S3Admin: client})
		if diags.HasError() {
			t.Fatalf("expected delete to succeed, got %v", diags)
		}
		if d.Id() != "" {
			t.Fatalf("expected empty id, got %q", d.Id())
		}
	})

	t.Run("delete/skips missing group and removes remaining", func(t *testing.T) {
		called := make(map[string]bool)
		client := iamMissingTargetAdminStub(t, func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/update-group-members") {
				t.Errorf("unexpected path %s %s", r.Method, r.URL.Path)
				http.NotFound(w, r)
				return
			}
			var req madmin.GroupAddRemove
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			called[req.Group] = true
			if req.Group == "group-a" {
				writeMadminStubError(w, http.StatusNotFound, "XMinioAdminNoSuchGroup", "group does not exist")
				return
			}
			w.WriteHeader(http.StatusOK)
		})

		d := res.TestResourceData()
		if err := d.Set("user", "test-user"); err != nil {
			t.Fatal(err)
		}
		if err := d.Set("groups", []string{"group-a", "group-b"}); err != nil {
			t.Fatal(err)
		}
		d.SetId("test-user")

		diags := res.DeleteContext(context.Background(), d, &S3MinioClient{S3Admin: client})
		if diags.HasError() {
			t.Fatalf("expected delete to succeed, got %v", diags)
		}
		if !called["group-a"] || !called["group-b"] {
			t.Fatalf("expected both groups to be processed, got %v", called)
		}
		if d.Id() != "" {
			t.Fatalf("expected empty id, got %q", d.Id())
		}
	})

	t.Run("delete/access denied errors", func(t *testing.T) {
		client := iamMissingTargetAdminStub(t, func(w http.ResponseWriter, r *http.Request) {
			writeMadminStubError(w, http.StatusForbidden, "AccessDenied", "Access Denied.")
		})

		d := res.TestResourceData()
		if err := d.Set("user", "test-user"); err != nil {
			t.Fatal(err)
		}
		if err := d.Set("groups", []string{"group-a"}); err != nil {
			t.Fatal(err)
		}
		d.SetId("test-user")

		diags := res.DeleteContext(context.Background(), d, &S3MinioClient{S3Admin: client})
		if !diags.HasError() {
			t.Fatal("expected error on access denied")
		}
	})
}

func testAccDeleteMinioGroup(t *testing.T, groupName string) {
	t.Helper()
	ctx := context.Background()
	admin := testAccClient().S3Admin

	desc, err := admin.GetGroupDescription(ctx, groupName)
	if err != nil {
		if isNoSuchGroupError(err) {
			return
		}
		t.Fatalf("reading group %s before delete: %v", groupName, err)
	}

	if len(desc.Members) > 0 {
		if rmErr := admin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
			Group:    groupName,
			Members:  desc.Members,
			IsRemove: true,
		}); rmErr != nil && !isNoSuchGroupError(rmErr) {
			t.Fatalf("removing members from group %s: %v", groupName, rmErr)
		}
	}

	rmErr := admin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
		Group:    groupName,
		Members:  []string{},
		IsRemove: true,
	})
	if rmErr != nil && !isNoSuchGroupError(rmErr) {
		if desc.Policy != "" {
			if _, detErr := admin.DetachPolicy(ctx, madmin.PolicyAssociationReq{
				Policies: strings.Split(desc.Policy, ","),
				Group:    groupName,
			}); detErr != nil && !isNoSuchGroupError(detErr) {
				t.Fatalf("detaching policies from group %s before delete: %v", groupName, detErr)
			}
			rmErr = admin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
				Group:    groupName,
				Members:  []string{},
				IsRemove: true,
			})
		}
		if rmErr != nil && !isNoSuchGroupError(rmErr) {
			t.Fatalf("deleting group %s outside Terraform: %v", groupName, rmErr)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, checkErr := admin.GetGroupDescription(ctx, groupName)
		if isNoSuchGroupError(checkErr) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("group %s still exists after external deletion", groupName)
}

func testAccDeleteMinioUser(t *testing.T, userName string) {
	t.Helper()
	ctx := context.Background()
	admin := testAccClient().S3Admin

	if err := admin.RemoveUser(ctx, userName); err != nil && !isNoSuchUserError(err) {
		t.Fatalf("deleting user %s outside Terraform: %v", userName, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, checkErr := admin.GetUserInfo(ctx, userName)
		if isNoSuchUserError(checkErr) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("user %s still exists after external deletion", userName)
}

func testAccCaptureResourceID(name string, out *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource %s not found in state", name)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("resource %s has empty ID", name)
		}
		*out = rs.Primary.ID
		return nil
	}
}

func testAccCheckMinioGroupMemberExists(groupName, userName string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		desc, err := testAccClient().S3Admin.GetGroupDescription(context.Background(), groupName)
		if err != nil {
			return fmt.Errorf("reading group %s: %w", groupName, err)
		}
		for _, member := range desc.Members {
			if member == userName {
				return nil
			}
		}
		return errors.New("expected user " + userName + " in group " + groupName + " members")
	}
}

func testAccCheckMinioGroupPolicyAttached(groupName, policyName string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		// Assert against GetGroupDescription directly: minioReadGroupPolicies
		// is production code under test, and an expectation built by it only
		// proves the code agrees with itself.
		desc, err := testAccClient().S3Admin.GetGroupDescription(context.Background(), groupName)
		if err != nil {
			return fmt.Errorf("reading group %s: %w", groupName, err)
		}
		for _, attached := range strings.Split(desc.Policy, ",") {
			if attached == policyName {
				return nil
			}
		}
		return errors.New("expected policy " + policyName + " on group " + groupName)
	}
}
