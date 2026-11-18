package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/madmin-go/v4"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// A tier list that briefly lags behind AddTier must not drop a tier that was
// just created (issue #1309): the read right after create polls ListTiers
// until the tier shows up, while a refresh still treats a missing tier as
// deleted outside Terraform. The stub answers with an empty list for the
// first omitListings requests to reproduce that window.
func TestMinioReadILMTier(t *testing.T) {
	tierJSON := `[{"version":"v1","type":"minio","name":"TFTIER","minio":{"endpoint":"http://remote:9000","accesskey":"AKIAEXAMPLE","secretkey":"secret","bucket":"cold-storage","prefix":"tier/"}}]`

	cases := []struct {
		name         string
		newResource  bool
		omitListings int
		ctxTimeout   time.Duration
		wantErr      bool
		wantGone     bool
		wantCalls    int
		minCallsOnly bool
	}{
		{
			name:      "listed tier stays in state",
			wantCalls: 1,
		},
		{
			name:         "tier deleted outside Terraform is removed from state",
			omitListings: -1,
			wantGone:     true,
			wantCalls:    1,
		},
		{
			name:         "tier missing right after create is retried until listed",
			newResource:  true,
			omitListings: 2,
			wantCalls:    3,
		},
		{
			name:         "tier still missing right after create is an error, not a removal",
			newResource:  true,
			omitListings: -1,
			ctxTimeout:   1500 * time.Millisecond,
			wantErr:      true,
			wantCalls:    2,
			minCallsOnly: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			calls := 0

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if !strings.Contains(r.URL.Path, "/tier") {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				mu.Lock()
				calls++
				omit := tc.omitListings < 0 || calls <= tc.omitListings
				mu.Unlock()

				if omit {
					fmt.Fprint(w, "[]")
					return
				}
				fmt.Fprint(w, tierJSON)
			}))
			defer srv.Close()

			adminClient, err := madmin.NewWithOptions(strings.TrimPrefix(srv.URL, "http://"), &madmin.Options{
				Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
				Secure: false,
			})
			if err != nil {
				t.Fatalf("creating admin client: %v", err)
			}

			d := schema.TestResourceDataRaw(t, resourceMinioILMTier().Schema, map[string]interface{}{
				"name":   "TFTIER",
				"type":   "minio",
				"bucket": "cold-storage",
			})
			d.SetId("TFTIER")
			if tc.newResource {
				d.MarkNewResource()
			}

			ctx := context.Background()
			if tc.ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.ctxTimeout)
				defer cancel()
			}

			diags := minioReadILMTier(ctx, d, &S3MinioClient{S3Admin: adminClient})

			if tc.wantErr != diags.HasError() {
				t.Fatalf("wantErr %v, got diagnostics: %v", tc.wantErr, diags)
			}
			if gone := d.Id() == ""; gone != tc.wantGone {
				t.Fatalf("wantGone %v, id is %q", tc.wantGone, d.Id())
			}

			mu.Lock()
			defer mu.Unlock()
			if tc.minCallsOnly {
				if calls < tc.wantCalls {
					t.Errorf("expected at least %d ListTiers calls, got %d", tc.wantCalls, calls)
				}
			} else if calls != tc.wantCalls {
				t.Errorf("expected %d ListTiers calls, got %d", tc.wantCalls, calls)
			}
			if tc.wantErr || tc.wantGone {
				return
			}
			for attr, want := range map[string]string{
				"type":                      "minio",
				"bucket":                    "cold-storage",
				"prefix":                    "tier/",
				"endpoint":                  "http://remote:9000",
				"minio_config.0.access_key": "AKIAEXAMPLE",
			} {
				if got := d.Get(attr); got != want {
					t.Errorf("expected %s = %q, got %q", attr, want, got)
				}
			}
		})
	}
}
