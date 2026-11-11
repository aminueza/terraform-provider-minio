package minio

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestMinioUpdateILMPolicyToAbortOnlyRemovesOldLifecycle(t *testing.T) {
	const expirationLifecycle = `<LifecycleConfiguration><Rule><ID>rule</ID><Status>Enabled</Status><Filter><Prefix></Prefix></Filter><Expiration><Days>30</Days></Expiration></Rule></LifecycleConfiguration>`
	const noLifecycle = `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchLifecycleConfiguration</Code><Message>error</Message></Error>`

	cases := []struct {
		name          string
		hasLifecycle  bool
		deleteStatus  int
		wantDeletes   int
		wantErr       string
		wantLifecycle bool
	}{
		{name: "the old expiration rule is removed", hasLifecycle: true, deleteStatus: http.StatusNoContent, wantDeletes: 1},
		{name: "a bucket without lifecycle is left alone", wantDeletes: 0},
		{name: "a denied removal is reported", hasLifecycle: true, deleteStatus: http.StatusForbidden, wantDeletes: 1, wantErr: "removing lifecycle", wantLifecycle: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			hasLifecycle := tc.hasLifecycle
			deletes := 0

			client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/xml")
				switch {
				case r.Method == http.MethodHead:
					w.WriteHeader(http.StatusOK)
				case r.Method == http.MethodGet && r.URL.Query().Has("lifecycle"):
					if !hasLifecycle {
						w.WriteHeader(http.StatusNotFound)
						_, _ = fmt.Fprint(w, noLifecycle)
						return
					}
					w.WriteHeader(http.StatusOK)
					_, _ = fmt.Fprint(w, expirationLifecycle)
				case r.Method == http.MethodDelete && r.URL.Query().Has("lifecycle"):
					deletes++
					if tc.deleteStatus != http.StatusNoContent {
						w.WriteHeader(tc.deleteStatus)
						_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>error</Message></Error>`)
						return
					}
					hasLifecycle = false
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusBadRequest)
				}
			})

			d := schema.TestResourceDataRaw(t, resourceMinioILMPolicy().Schema, map[string]interface{}{
				"bucket": "test-bucket",
				"rule": []interface{}{map[string]interface{}{
					"id":                                "rule",
					"status":                            "Enabled",
					"abort_incomplete_multipart_upload": []interface{}{map[string]interface{}{"days_after_initiation": "7d"}},
				}},
			})
			d.SetId("test-bucket")

			diags := minioCreateILMPolicy(context.Background(), d, &S3MinioClient{S3Client: client})

			mu.Lock()
			defer mu.Unlock()
			if deletes != tc.wantDeletes {
				t.Errorf("got %d lifecycle removals, want %d", deletes, tc.wantDeletes)
			}
			if hasLifecycle != tc.wantLifecycle {
				t.Errorf("bucket has lifecycle = %v, want %v", hasLifecycle, tc.wantLifecycle)
			}

			if tc.wantErr != "" {
				if !diags.HasError() || !strings.Contains(diags[0].Summary, tc.wantErr) {
					t.Fatalf("got %v, want an error containing %q", diags, tc.wantErr)
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("update failed: %v", diags)
			}
			if got := d.Get("rule.0.abort_incomplete_multipart_upload.0.days_after_initiation").(string); got != "7d" {
				t.Errorf("abort rule in state = %q, want %q", got, "7d")
			}
			if got := d.Get("rule.0.expiration").(string); got != "" {
				t.Errorf("expiration in state = %q, want it empty", got)
			}
		})
	}
}
