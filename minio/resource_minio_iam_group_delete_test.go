package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestMinioDeleteGroupDoesNotAttachPolicy(t *testing.T) {
	const notFound = `{"Code":"XMinioAdminNoSuchGroup","Message":"The specified group does not exist.","Resource":"/minio/admin/v3/group"}`

	cases := []struct {
		name              string
		forceDestroy      bool
		members           []string
		wantErr           string
		wantUpdateMembers int
	}{
		{
			name:              "empty group",
			forceDestroy:      false,
			members:           []string{},
			wantUpdateMembers: 1,
		},
		{
			name:              "members with force_destroy false",
			forceDestroy:      false,
			members:           []string{"alice"},
			wantErr:           "group still has 1 member(s)",
			wantUpdateMembers: 0,
		},
		{
			name:              "members with force_destroy true",
			forceDestroy:      true,
			members:           []string{"alice"},
			wantUpdateMembers: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			attachPolicyCalls := 0
			updateMembersCalls := 0
			removed := false

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.Contains(r.URL.Path, "policy"):
					attachPolicyCalls++
					w.WriteHeader(http.StatusOK)
					_, _ = fmt.Fprint(w, `{}`)
				case strings.HasSuffix(r.URL.Path, "/update-group-members"):
					updateMembersCalls++
					if len(tc.members) == 0 || updateMembersCalls >= 2 {
						removed = true
					}
					w.WriteHeader(http.StatusOK)
				case strings.HasSuffix(r.URL.Path, "/group"):
					if removed {
						w.WriteHeader(http.StatusNotFound)
						_, _ = fmt.Fprint(w, notFound)
						return
					}
					membersJSON := `[]`
					if len(tc.members) > 0 {
						quoted := make([]string, len(tc.members))
						for i, m := range tc.members {
							quoted[i] = fmt.Sprintf("%q", m)
						}
						membersJSON = "[" + strings.Join(quoted, ",") + "]"
					}
					w.WriteHeader(http.StatusOK)
					_, _ = fmt.Fprintf(w, `{"name":"tfacc-group","status":"enabled","members":%s,"policy":""}`, membersJSON)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			admin, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
				Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
				Secure: false,
			})
			if err != nil {
				t.Fatalf("creating the admin client: %s", err)
			}

			d := resourceMinioIAMGroup().TestResourceData()
			d.SetId("tfacc-group")
			if err := d.Set("name", "tfacc-group"); err != nil {
				t.Fatalf("setting name: %s", err)
			}
			if err := d.Set("force_destroy", tc.forceDestroy); err != nil {
				t.Fatalf("setting force_destroy: %s", err)
			}

			diags := minioDeleteGroup(context.Background(), d, &S3MinioClient{S3Admin: admin})

			if tc.wantErr == "" && diags.HasError() {
				t.Fatalf("minioDeleteGroup() returned unexpected error diagnostics: %v", diags)
			}
			if tc.wantErr != "" {
				if !diags.HasError() || !strings.Contains(diags[0].Detail+diags[0].Summary, tc.wantErr) {
					t.Fatalf("minioDeleteGroup() = %v, want error containing %q", diags, tc.wantErr)
				}
			}
			if attachPolicyCalls != 0 {
				t.Errorf("policy attach endpoint called %d time(s), want 0", attachPolicyCalls)
			}
			if updateMembersCalls != tc.wantUpdateMembers {
				t.Errorf("update-group-members called %d time(s), want %d", updateMembersCalls, tc.wantUpdateMembers)
			}
		})
	}
}
