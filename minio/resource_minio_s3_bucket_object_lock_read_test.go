package minio

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestReadObjectLockConfigurationClearsRemovedRule(t *testing.T) {
	cases := []struct {
		name      string
		lockBody  string
		wantRules int
		wantDays  int
	}{
		{
			name:      "default retention removed outside Terraform",
			lockBody:  `<ObjectLockConfiguration><ObjectLockEnabled>Enabled</ObjectLockEnabled></ObjectLockConfiguration>`,
			wantRules: 0,
		},
		{
			name:      "default retention still set",
			lockBody:  `<ObjectLockConfiguration><ObjectLockEnabled>Enabled</ObjectLockEnabled><Rule><DefaultRetention><Mode>GOVERNANCE</Mode><Days>7</Days></DefaultRetention></Rule></ObjectLockConfiguration>`,
			wantRules: 1,
			wantDays:  7,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				if _, ok := r.URL.Query()["object-lock"]; ok {
					w.Header().Set("Content-Type", "application/xml")
					_, _ = fmt.Fprint(w, tc.lockBody)
					return
				}
				w.WriteHeader(http.StatusOK)
			})

			d := schema.TestResourceDataRaw(t, resourceMinioS3BucketObjectLockConfiguration().Schema, map[string]interface{}{
				"bucket": "locked-bucket",
				"rule": []interface{}{map[string]interface{}{
					"default_retention": []interface{}{map[string]interface{}{
						"mode": "COMPLIANCE",
						"days": 30,
					}},
				}},
			})
			d.SetId("locked-bucket")

			diags := minioReadObjectLockConfiguration(context.Background(), d, &S3MinioClient{S3Client: client})
			if diags.HasError() {
				t.Fatalf("unexpected error: %v", diags)
			}
			if d.Id() != "locked-bucket" {
				t.Fatalf("resource dropped from state")
			}
			rules := d.Get("rule").([]interface{})
			if len(rules) != tc.wantRules {
				t.Fatalf("rule = %v, want %d entries", rules, tc.wantRules)
			}
			if tc.wantRules == 1 {
				if got := d.Get("rule.0.default_retention.0.days").(int); got != tc.wantDays {
					t.Errorf("days = %d, want %d", got, tc.wantDays)
				}
				if got := d.Get("rule.0.default_retention.0.mode").(string); got != "GOVERNANCE" {
					t.Errorf("mode = %q, want GOVERNANCE", got)
				}
			}
		})
	}
}
