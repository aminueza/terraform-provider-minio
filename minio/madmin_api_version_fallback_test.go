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

// TestMadminAdminAPIVersionCompatibility is a regression test for #1071: the
// provider must work against MinIO servers that speak admin API v4 as well as
// older servers that only speak v3. madmin-go/v4 sends /v4 requests and is
// expected to transparently retry with /v3 when the server answers
// 426 Upgrade Required, so upgrading the client library must not break
// existing deployments.
func TestMadminAdminAPIVersionCompatibility(t *testing.T) {
	const quotaJSON = `{"size":1024,"quotatype":"hard"}`

	// The madmin client picks its starting admin API version from the
	// MADMIN_API_VERSION environment variable at package init (the CI compose
	// fixture pins v3 to match the pinned MinIO image), so derive the
	// expectations from the active version instead of assuming v4.
	activeVersion := madmin.AdminAPIVersion

	cases := []struct {
		name          string
		skip          string
		handler       func(mu *sync.Mutex, paths *[]string) http.HandlerFunc
		wantPathHits  []string
		wantQuotaSize uint64
	}{
		{
			name: "server answering the active version directly",
			handler: func(mu *sync.Mutex, paths *[]string) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					*paths = append(*paths, r.URL.Path)
					mu.Unlock()
					if strings.Contains(r.URL.Path, "/"+activeVersion+"/get-bucket-quota") {
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprint(w, quotaJSON)
						return
					}
					w.WriteHeader(http.StatusNotFound)
				}
			},
			wantPathHits:  []string{"/minio/admin/" + activeVersion + "/get-bucket-quota"},
			wantQuotaSize: 1024,
		},
		{
			name: "v3-only server triggers 426 fallback",
			skip: func() string {
				if activeVersion != "v4" {
					return "fallback requires the client to start at v4; MADMIN_API_VERSION=" + activeVersion
				}
				return ""
			}(),
			handler: func(mu *sync.Mutex, paths *[]string) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					*paths = append(*paths, r.URL.Path)
					mu.Unlock()
					if strings.Contains(r.URL.Path, "/v4/") {
						w.WriteHeader(http.StatusUpgradeRequired)
						return
					}
					if strings.Contains(r.URL.Path, "/v3/get-bucket-quota") {
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprint(w, quotaJSON)
						return
					}
					w.WriteHeader(http.StatusNotFound)
				}
			},
			wantPathHits: []string{
				"/minio/admin/v4/get-bucket-quota",
				"/minio/admin/v3/get-bucket-quota",
			},
			wantQuotaSize: 1024,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip != "" {
				t.Skip(tc.skip)
			}
			var mu sync.Mutex
			var paths []string

			srv := httptest.NewServer(tc.handler(&mu, &paths))
			defer srv.Close()

			host := strings.TrimPrefix(srv.URL, "http://")
			adminClient, err := madmin.NewWithOptions(host, &madmin.Options{
				Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
				Secure: false,
			})
			if err != nil {
				t.Fatalf("creating admin client: %v", err)
			}

			quota, err := adminClient.GetBucketQuota(context.Background(), "test-bucket")
			if err != nil {
				t.Fatalf("GetBucketQuota failed: %v", err)
			}
			if quota.Size != tc.wantQuotaSize {
				t.Errorf("expected quota size %d, got %d", tc.wantQuotaSize, quota.Size)
			}

			mu.Lock()
			defer mu.Unlock()
			if len(paths) != len(tc.wantPathHits) {
				t.Fatalf("expected %d requests %v, got %d: %v", len(tc.wantPathHits), tc.wantPathHits, len(paths), paths)
			}
			for i, want := range tc.wantPathHits {
				if paths[i] != want {
					t.Errorf("request %d: expected path %q, got %q", i, want, paths[i])
				}
			}
		})
	}
}
