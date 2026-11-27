package minio

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// RustFS answers a missing user and a missing group with the same body, so
// these tests check that each call site attributes it to the right object.
// See issue #1485.
const rustfsNoSuchResource = `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchResource</Code><Message>object does not exist</Message></Error>`

func writeRustfsMissing(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, rustfsNoSuchResource)
}

func newAmbiguousStubAdmin(t *testing.T, handler http.HandlerFunc) *madmin.AdminClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
		Creds: credentials.NewStaticV4("accesskey", "secretkey", ""),
	})
	if err != nil {
		t.Fatalf("creating admin client: %v", err)
	}
	return admin
}

func TestUserPolicyAttachmentReadDropsMissingUser(t *testing.T) {
	admin := newAmbiguousStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/user-info") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeRustfsMissing(w)
	})

	d := schema.TestResourceDataRaw(t, resourceMinioIAMUserPolicyAttachment().Schema, map[string]interface{}{
		"user_name":   "alice",
		"policy_name": "readonly",
	})
	d.SetId("alice-readonly")

	if diags := doMinioReadUserPolicyAttachment(context.Background(), d, &S3MinioClient{S3Admin: admin}, "alice", "readonly"); diags.HasError() {
		t.Fatalf("read failed: %v", diags)
	}
	if d.Id() != "" {
		t.Errorf("id = %q, want the attachment dropped from state once the user is gone", d.Id())
	}
}

func TestUserGroupMembershipDeleteKeepsGoingPastMissingGroup(t *testing.T) {
	var mu sync.Mutex
	var groups []string
	admin := newAmbiguousStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/update-group-members") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var req madmin.GroupAddRemove
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding body: %v", err)
		}
		mu.Lock()
		groups = append(groups, req.Group)
		first := len(groups) == 1
		mu.Unlock()
		// The first group answered is the missing one, whichever the set
		// yields first, so a loop that stops on it always skips the other.
		if first {
			writeRustfsMissing(w)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	res := resourceMinioIAMUserGroupMembership()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"user":   "alice",
		"groups": []interface{}{"devs", "ops"},
	})
	d.SetId("alice")

	if diags := res.DeleteContext(context.Background(), d, &S3MinioClient{S3Admin: admin}); diags.HasError() {
		t.Fatalf("delete failed: %v", diags)
	}
	sort.Strings(groups)
	if strings.Join(groups, ",") != "devs,ops" {
		t.Errorf("groups the user was removed from = %v, want [devs ops]: a missing group must not stop the loop", groups)
	}
	if d.Id() != "" {
		t.Errorf("id = %q, want empty after delete", d.Id())
	}
}

func TestGroupMembershipRemovalHitMissingUser(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		groupExists bool
		wantUser    bool
		wantLookup  bool
	}{
		{
			name:        "NoSuchResource with the group present is a missing user",
			err:         madmin.ErrorResponse{Code: "NoSuchResource", StatusCode: http.StatusNotFound},
			groupExists: true,
			wantUser:    true,
			wantLookup:  true,
		},
		{
			name:       "NoSuchResource with the group gone is not a missing user",
			err:        madmin.ErrorResponse{Code: "NoSuchResource", StatusCode: http.StatusNotFound},
			wantLookup: true,
		},
		{
			name:     "MinIO's missing user code needs no lookup",
			err:      madmin.ErrorResponse{Code: "XMinioAdminNoSuchUser"},
			wantUser: true,
		},
		{
			name: "MinIO's missing group code is not a missing user",
			err:  madmin.ErrorResponse{Code: "XMinioAdminNoSuchGroup"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var lookups int
			admin := newAmbiguousStubAdmin(t, func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/group") {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				lookups++
				if !tc.groupExists {
					writeRustfsMissing(w)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(madmin.GroupDesc{Name: "devs", Status: "enabled"})
			})

			if got := groupMembershipRemovalHitMissingUser(context.Background(), admin, "devs", tc.err); got != tc.wantUser {
				t.Errorf("missing user = %v, want %v", got, tc.wantUser)
			}
			if (lookups > 0) != tc.wantLookup {
				t.Errorf("group lookups = %d, want a lookup: %v", lookups, tc.wantLookup)
			}
		})
	}
}
