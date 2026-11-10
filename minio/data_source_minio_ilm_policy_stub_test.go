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
		wantRules      int
		wantGapWarning bool
	}{
		{
			name:      "rules are reported",
			status:    http.StatusOK,
			body:      `<LifecycleConfiguration><Rule><ID>expire</ID><Status>Enabled</Status><Filter><Prefix>logs/</Prefix></Filter><Expiration><Days>7</Days></Expiration></Rule></LifecycleConfiguration>`,
			wantRules: 1,
		},
		{
			name:   "a bucket without lifecycle rules reads as empty",
			status: http.StatusNotFound,
			body:   dataSourceBucketStubXML("NoSuchLifecycleConfiguration", "The lifecycle configuration does not exist"),
		},
		{
			name:   "a 204 without lifecycle rules reads as empty",
			status: http.StatusNoContent,
		},
		{
			name:    "a missing bucket is an error",
			status:  http.StatusNotFound,
			body:    dataSourceBucketStubXML("NoSuchBucket", "The specified bucket does not exist"),
			wantErr: "reading bucket lifecycle",
		},
		{
			name:       "a denied read is an error even with s3_compat_mode",
			compatMode: true,
			status:     http.StatusForbidden,
			body:       dataSourceBucketStubXML("AccessDenied", "Access Denied."),
			wantErr:    "reading bucket lifecycle",
		},
		{
			name:    "a server error is an error",
			status:  http.StatusInternalServerError,
			body:    dataSourceBucketStubXML("InternalError", "We encountered an internal error, please try again."),
			wantErr: "reading bucket lifecycle",
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
			wantErr: "reading bucket lifecycle",
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
					t.Fatalf("read succeeded with %d rules, want an error containing %q", len(d.Get("rules").([]interface{})), tc.wantErr)
				}
				if !strings.Contains(diags[0].Summary, tc.wantErr) {
					t.Fatalf("error summary = %q, want it to contain %q", diags[0].Summary, tc.wantErr)
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("read failed: %v", diags)
			}

			if got := len(d.Get("rules").([]interface{})); got != tc.wantRules {
				t.Errorf("got %d rules, want %d", got, tc.wantRules)
			}
			if tc.wantRules > 0 {
				if got := d.Get("rules.0.expiration_days").(int); got != 7 {
					t.Errorf("rules.0.expiration_days = %d, want 7", got)
				}
				if got := d.Get("rules.0.prefix").(string); got != "logs/" {
					t.Errorf("rules.0.prefix = %q, want %q", got, "logs/")
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
