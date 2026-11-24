package minio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const importStubNotImplemented = `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NotImplemented</Code><Message>This operation is not implemented.</Message></Error>`

func TestBucketImportStateAgainstStubBackend(t *testing.T) {
	cases := []struct {
		name         string
		bucketExists bool
		policyStatus int
		policyBody   string
		wantErr      string
		wantACL      string
	}{
		{
			name:         "a bucket with a public-read policy imports with that acl",
			bucketExists: true,
			policyStatus: http.StatusOK,
			policyBody:   `{"Version":"2012-10-17","Statement":[{"Action":["s3:GetBucketLocation","s3:ListBucket"],"Effect":"Allow","Principal":{"AWS":["*"]},"Resource":["arn:aws:s3:::test-bucket"],"Sid":"ListBucketActions"},{"Action":["s3:GetObject"],"Effect":"Allow","Principal":{"AWS":["*"]},"Resource":["arn:aws:s3:::test-bucket/*"],"Sid":"ReadObjectActions"}]}`,
			wantACL:      "public-read",
		},
		{
			name:         "a backend without bucket policies imports the bucket as private",
			bucketExists: true,
			policyStatus: http.StatusNotImplemented,
			policyBody:   importStubNotImplemented,
			wantACL:      "private",
		},
		{
			name:         "a policy read that is denied still fails the import",
			bucketExists: true,
			policyStatus: http.StatusForbidden,
			policyBody:   `<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>Access Denied.</Message></Error>`,
			wantErr:      "error importing Minio S3 bucket policy",
		},
		{
			name:         "a missing bucket is reported by name",
			bucketExists: false,
			wantErr:      `cannot import bucket "test-bucket": the bucket does not exist`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var policyCalls int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !tc.bucketExists {
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchBucket</Code><Message>The specified bucket does not exist</Message></Error>`)
					return
				}
				query := r.URL.Query()
				switch {
				case query.Has("policy"):
					policyCalls++
					if tc.policyStatus != http.StatusOK {
						w.Header().Set("Content-Type", "application/xml")
					}
					w.WriteHeader(tc.policyStatus)
					_, _ = fmt.Fprint(w, tc.policyBody)
				case query.Has("tagging"), query.Has("object-lock"):
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusNotImplemented)
					_, _ = fmt.Fprint(w, importStubNotImplemented)
				default:
					w.WriteHeader(http.StatusOK)
				}
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

			d := schema.TestResourceDataRaw(t, resourceMinioBucket().Schema, map[string]interface{}{})
			d.SetId("test-bucket")

			meta := &S3MinioClient{S3Client: s3Client, MaxRetries: 1, RetryDelayMs: 1}
			imported, err := resourceMinioS3BucketImportState(context.Background(), d, meta)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("import succeeded, want an error containing %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("import error = %q, want it to contain %q", err.Error(), tc.wantErr)
				}
				if !tc.bucketExists && policyCalls != 0 {
					t.Errorf("the policy was requested %d times for a missing bucket, want 0", policyCalls)
				}
				return
			}

			if err != nil {
				t.Fatalf("import failed: %v", err)
			}
			if len(imported) != 1 {
				t.Fatalf("import returned %d resources, want 1", len(imported))
			}
			if got := imported[0].Get("acl").(string); got != tc.wantACL {
				t.Errorf("acl = %q, want %q", got, tc.wantACL)
			}
			if got := imported[0].Id(); got != "test-bucket" {
				t.Errorf("id = %q, want %q", got, "test-bucket")
			}
		})
	}
}
