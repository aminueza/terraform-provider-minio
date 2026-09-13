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

// TestResourceMinioS3BucketImportState_policyNotImplemented verifies that
// importing a bucket succeeds as "private" when the backend doesn't
// implement the bucket policy API (e.g. Backblaze B2, which responds
// 501/NotImplemented to GetBucketPolicy) instead of the "NoSuchBucketPolicy"
// response minio-go already treats as "no policy". Before this fix, any
// non-"NoSuchBucketPolicy" error from GetBucketPolicy - including this one -
// hard-failed the import itself, even though normal Create/Read/Update never
// call this API at all and already handle the same backend class gracefully
// (see removeBucketPolicy).
func TestResourceMinioS3BucketImportState_policyNotImplemented(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		code       string
	}{
		{name: "501 NotImplemented", statusCode: http.StatusNotImplemented, code: "NotImplemented"},
		{name: "405 MethodNotAllowed", statusCode: http.StatusMethodNotAllowed, code: "MethodNotAllowed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Query().Has("policy"):
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(tc.statusCode)
					_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>This API call is not supported.</Message></Error>`, tc.code)
				case r.URL.Query().Has("tagging"):
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusOK)
					_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Tagging><TagSet></TagSet></Tagging>`)
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

			d := schema.TestResourceDataRaw(t, resourceMinioBucket().Schema, map[string]interface{}{"bucket": "Nextcloud-Fastnetserv"})
			d.SetId("Nextcloud-Fastnetserv")

			meta := &S3MinioClient{S3Client: s3Client}

			results, err := resourceMinioS3BucketImportState(context.Background(), d, meta)
			if err != nil {
				t.Fatalf("import returned error: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}

			if got := results[0].State().Attributes["acl"]; got != "private" {
				t.Errorf("acl = %q, want %q", got, "private")
			}
		})
	}
}
