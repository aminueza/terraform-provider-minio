package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type dataSourceBucketStubAnswer struct {
	status int
	body   string
}

func dataSourceBucketStubXML(code, message string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>%s</Message></Error>`, code, message)
}

func TestDataSourceMinioS3BucketReadAgainstStubBackend(t *testing.T) {
	notImplemented := dataSourceBucketStubAnswer{http.StatusNotImplemented, dataSourceBucketStubXML("NotImplemented", "This operation is not implemented.")}
	accessDenied := dataSourceBucketStubAnswer{http.StatusForbidden, dataSourceBucketStubXML("AccessDenied", "Access Denied.")}
	const policyJSON = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::test-bucket/*"]}]}`

	cases := []struct {
		name           string
		compatMode     bool
		answers        map[string]dataSourceBucketStubAnswer
		wantErr        string
		wantVersioning bool
		wantObjectLock bool
		wantPolicy     string
		wantWarnings   []string
	}{
		{
			name:       "a MinIO bucket reports what the server answers",
			compatMode: false,
			answers: map[string]dataSourceBucketStubAnswer{
				"versioning":  {http.StatusOK, `<VersioningConfiguration><Status>Enabled</Status></VersioningConfiguration>`},
				"object-lock": {http.StatusNotFound, dataSourceBucketStubXML("ObjectLockConfigurationNotFoundError", "Object Lock configuration does not exist for this bucket")},
				"policy":      {http.StatusOK, policyJSON},
			},
			wantVersioning: true,
			wantObjectLock: false,
			wantPolicy:     policyJSON,
		},
		{
			name:       "with s3_compat_mode a missing feature is a warning that names the attribute",
			compatMode: true,
			answers: map[string]dataSourceBucketStubAnswer{
				"versioning":  notImplemented,
				"object-lock": notImplemented,
				"policy":      notImplemented,
			},
			wantWarnings: []string{"versioning_enabled", "object_lock_enabled", "policy"},
		},
		{
			name:       "without s3_compat_mode a missing feature is an error",
			compatMode: false,
			answers: map[string]dataSourceBucketStubAnswer{
				"versioning":  notImplemented,
				"object-lock": notImplemented,
				"policy":      notImplemented,
			},
			wantErr: "reading bucket versioning",
		},
		{
			name:       "a denied policy read is an error even with s3_compat_mode",
			compatMode: true,
			answers: map[string]dataSourceBucketStubAnswer{
				"versioning":  {http.StatusOK, `<VersioningConfiguration></VersioningConfiguration>`},
				"object-lock": {http.StatusNotFound, dataSourceBucketStubXML("ObjectLockConfigurationNotFoundError", "Object Lock configuration does not exist for this bucket")},
				"policy":      accessDenied,
			},
			wantErr: "reading bucket policy",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query()
				if query.Has("location") {
					w.Header().Set("Content-Type", "application/xml")
					_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><LocationConstraint>us-east-1</LocationConstraint>`)
					return
				}
				for key, answer := range tc.answers {
					if query.Has(key) {
						w.Header().Set("Content-Type", "application/xml")
						w.WriteHeader(answer.status)
						_, _ = fmt.Fprint(w, answer.body)
						return
					}
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			s3Client, err := minio.New(strings.TrimPrefix(srv.URL, "http://"), &minio.Options{
				Creds:  credentials.NewStaticV4("accesskey", "secretkey", ""),
				Secure: false,
				Region: "us-east-1",
			})
			if err != nil {
				t.Fatalf("creating S3 client: %v", err)
			}

			d := schema.TestResourceDataRaw(t, dataSourceMinioS3Bucket().Schema, map[string]interface{}{"bucket": "test-bucket"})
			diags := dataSourceMinioS3BucketRead(context.Background(), d, &S3MinioClient{S3Client: s3Client, S3CompatMode: tc.compatMode})

			if tc.wantErr != "" {
				if !diags.HasError() {
					t.Fatalf("read succeeded, want an error containing %q", tc.wantErr)
				}
				if !strings.Contains(diags[0].Summary, tc.wantErr) {
					t.Fatalf("error summary = %q, want it to contain %q", diags[0].Summary, tc.wantErr)
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("read failed: %v", diags)
			}

			if got := d.Get("versioning_enabled").(bool); got != tc.wantVersioning {
				t.Errorf("versioning_enabled = %v, want %v", got, tc.wantVersioning)
			}
			if got := d.Get("object_lock_enabled").(bool); got != tc.wantObjectLock {
				t.Errorf("object_lock_enabled = %v, want %v", got, tc.wantObjectLock)
			}
			if got := d.Get("policy").(string); got != tc.wantPolicy {
				t.Errorf("policy = %q, want %q", got, tc.wantPolicy)
			}
			if got := d.Get("region").(string); got != "us-east-1" {
				t.Errorf("region = %q, want %q", got, "us-east-1")
			}

			if len(diags) != len(tc.wantWarnings) {
				t.Fatalf("got %d diagnostics, want %d warnings: %v", len(diags), len(tc.wantWarnings), diags)
			}
			for i, attribute := range tc.wantWarnings {
				if diags[i].Severity != diag.Warning {
					t.Errorf("diagnostic %d has severity %v, want a warning", i, diags[i].Severity)
				}
				if !strings.Contains(diags[i].Detail, attribute) || !strings.Contains(diags[i].Detail, "s3_compat_mode") {
					t.Errorf("warning %d = %q, want it to name %q and s3_compat_mode", i, diags[i].Detail, attribute)
				}
			}
		})
	}
}
