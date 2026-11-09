package minio

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestDataSourceMinioILMPolicyReadAgainstStubBackend(t *testing.T) {
	cases := []struct {
		name           string
		compatMode     bool
		status         int
		body           string
		wantErr        string
		wantRules      []map[string]interface{}
		wantGapWarning bool
	}{
		{
			name:   "rules are reported",
			status: http.StatusOK,
			body: `<LifecycleConfiguration>` +
				`<Rule><ID>expire</ID><Status>Enabled</Status><Filter><Prefix>logs/</Prefix></Filter><Expiration><Days>30</Days></Expiration></Rule>` +
				`<Rule><ID>noncurrent</ID><Status>Disabled</Status><Filter><Prefix></Prefix></Filter><NoncurrentVersionExpiration><NoncurrentDays>7</NoncurrentDays></NoncurrentVersionExpiration></Rule>` +
				`</LifecycleConfiguration>`,
			wantRules: []map[string]interface{}{
				{"id": "expire", "status": "Enabled", "prefix": "logs/", "expiration_days": 30, "noncurrent_expiration_days": 0},
				{"id": "noncurrent", "status": "Disabled", "prefix": "", "expiration_days": 0, "noncurrent_expiration_days": 7},
			},
		},
		{
			name:   "a bucket without lifecycle rules reads as empty",
			status: http.StatusNotFound,
			body:   dataSourceBucketStubXML("NoSuchLifecycleConfiguration", "The lifecycle configuration does not exist"),
		},
		{
			name:    "a missing bucket is an error",
			status:  http.StatusNotFound,
			body:    dataSourceBucketStubXML("NoSuchBucket", "The specified bucket does not exist"),
			wantErr: "reading lifecycle",
		},
		{
			name:       "a denied read is an error even with s3_compat_mode",
			compatMode: true,
			status:     http.StatusForbidden,
			body:       dataSourceBucketStubXML("AccessDenied", "Access Denied."),
			wantErr:    "reading lifecycle",
		},
		{
			name:    "a server error is an error",
			status:  http.StatusInternalServerError,
			body:    dataSourceBucketStubXML("InternalError", "We encountered an internal error, please try again."),
			wantErr: "reading lifecycle",
		},
		{
			name:           "with s3_compat_mode a missing feature is a warning",
			compatMode:     true,
			status:         http.StatusNotImplemented,
			body:           dataSourceBucketStubXML("NotImplemented", "This operation is not implemented."),
			wantGapWarning: true,
		},
		{
			name:           "with s3_compat_mode a 405 is a missing feature",
			compatMode:     true,
			status:         http.StatusMethodNotAllowed,
			body:           dataSourceBucketStubXML("MethodNotAllowed", "The specified method is not allowed against this resource."),
			wantGapWarning: true,
		},
		{
			name:    "without s3_compat_mode a missing feature is an error",
			status:  http.StatusNotImplemented,
			body:    dataSourceBucketStubXML("NotImplemented", "This operation is not implemented."),
			wantErr: "reading lifecycle",
		},
		{
			name:    "without s3_compat_mode a 405 is an error",
			status:  http.StatusMethodNotAllowed,
			body:    dataSourceBucketStubXML("MethodNotAllowed", "The specified method is not allowed against this resource."),
			wantErr: "reading lifecycle",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				if !r.URL.Query().Has("lifecycle") {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})

			d := schema.TestResourceDataRaw(t, dataSourceMinioILMPolicy().Schema, map[string]interface{}{"bucket": "test-bucket"})
			diags := dataSourceMinioILMPolicyRead(context.Background(), d, &S3MinioClient{S3Client: client, S3CompatMode: tc.compatMode})

			if tc.wantErr != "" {
				if !diags.HasError() {
					t.Fatalf("read succeeded with rules %v, want an error containing %q", d.Get("rules"), tc.wantErr)
				}
				if !strings.Contains(diags[0].Summary, tc.wantErr) {
					t.Fatalf("error summary = %q, want it to contain %q", diags[0].Summary, tc.wantErr)
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("read failed: %v", diags)
			}

			rules := d.Get("rules").([]interface{})
			if len(rules) != len(tc.wantRules) {
				t.Fatalf("got %d rules %v, want %d", len(rules), rules, len(tc.wantRules))
			}
			for i, want := range tc.wantRules {
				got := rules[i].(map[string]interface{})
				for k, v := range want {
					if got[k] != v {
						t.Errorf("rules[%d].%s = %v, want %v", i, k, got[k], v)
					}
				}
			}

			if !tc.wantGapWarning {
				if len(diags) != 0 {
					t.Fatalf("got diagnostics %v, want none", diags)
				}
				return
			}
			if len(diags) != 1 || diags[0].Severity != diag.Warning {
				t.Fatalf("got diagnostics %v, want one warning", diags)
			}
			if !strings.Contains(diags[0].Detail, "rules") || !strings.Contains(diags[0].Detail, "s3_compat_mode") {
				t.Errorf("warning = %q, want it to name rules and s3_compat_mode", diags[0].Detail)
			}
		})
	}
}
