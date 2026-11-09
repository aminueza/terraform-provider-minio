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

func TestDataSourceMinioS3BucketEncryptionReadAgainstStubBackend(t *testing.T) {
	cases := []struct {
		name           string
		compatMode     bool
		status         int
		body           string
		wantErr        string
		wantType       string
		wantKMSKeyID   string
		wantGapWarning bool
	}{
		{
			name:     "SSE-S3 is reported",
			status:   http.StatusOK,
			body:     `<ServerSideEncryptionConfiguration><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>AES256</SSEAlgorithm></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`,
			wantType: "AES256",
		},
		{
			name:         "SSE-KMS is reported with its key",
			status:       http.StatusOK,
			body:         `<ServerSideEncryptionConfiguration><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>aws:kms</SSEAlgorithm><KMSMasterKeyID>my-key</KMSMasterKeyID></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`,
			wantType:     "aws:kms",
			wantKMSKeyID: "my-key",
		},
		{
			name:   "a bucket without default encryption reads as empty",
			status: http.StatusNotFound,
			body:   dataSourceBucketStubXML("ServerSideEncryptionConfigurationNotFoundError", "The server side encryption configuration was not found"),
		},
		{
			name:    "a missing bucket is an error",
			status:  http.StatusNotFound,
			body:    dataSourceBucketStubXML("NoSuchBucket", "The specified bucket does not exist"),
			wantErr: "reading bucket encryption",
		},
		{
			name:       "a denied read is an error even with s3_compat_mode",
			compatMode: true,
			status:     http.StatusForbidden,
			body:       dataSourceBucketStubXML("AccessDenied", "Access Denied."),
			wantErr:    "reading bucket encryption",
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
			wantErr: "reading bucket encryption",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
				if !r.URL.Query().Has("encryption") {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})

			d := schema.TestResourceDataRaw(t, dataSourceMinioS3BucketEncryption().Schema, map[string]interface{}{"bucket": "test-bucket"})
			diags := dataSourceMinioS3BucketEncryptionRead(context.Background(), d, &S3MinioClient{S3Client: client, S3CompatMode: tc.compatMode})

			if tc.wantErr != "" {
				if !diags.HasError() {
					t.Fatalf("read succeeded with encryption_type %q, want an error containing %q", d.Get("encryption_type"), tc.wantErr)
				}
				if !strings.Contains(diags[0].Summary, tc.wantErr) {
					t.Fatalf("error summary = %q, want it to contain %q", diags[0].Summary, tc.wantErr)
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("read failed: %v", diags)
			}

			if got := d.Get("encryption_type").(string); got != tc.wantType {
				t.Errorf("encryption_type = %q, want %q", got, tc.wantType)
			}
			if got := d.Get("kms_master_key_id").(string); got != tc.wantKMSKeyID {
				t.Errorf("kms_master_key_id = %q, want %q", got, tc.wantKMSKeyID)
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
			if !strings.Contains(diags[0].Detail, "encryption_type") || !strings.Contains(diags[0].Detail, "s3_compat_mode") {
				t.Errorf("warning = %q, want it to name encryption_type and s3_compat_mode", diags[0].Detail)
			}
		})
	}
}
