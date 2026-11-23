package minio

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestDataSourceMinioS3ObjectReadTimestamps(t *testing.T) {
	const body = "hello world"

	t.Run("formats last_modified as RFC3339 and leaves zero expires empty", func(t *testing.T) {
		client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("ETag", `"5eb63bbbe01eeed093cb22bb8f5acdc3"`)
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(body))
			}
		})

		d := schema.TestResourceDataRaw(t, dataSourceMinioS3Object().Schema, map[string]interface{}{
			"bucket_name": "test-bucket",
			"object_name": "plain.txt",
		})
		if diags := dataSourceMinioS3ObjectRead(context.Background(), d, &S3MinioClient{S3Client: client}); diags.HasError() {
			t.Fatalf("dataSourceMinioS3ObjectRead failed: %v", diags)
		}

		if got := d.Get("last_modified").(string); got != "2006-01-02T15:04:05Z" {
			t.Fatalf("last_modified = %q, want %q", got, "2006-01-02T15:04:05Z")
		}
		if got := d.Get("expires").(string); got != "" {
			t.Fatalf("expires = %q, want empty string", got)
		}
		if got := d.Get("content").(string); got != body {
			t.Fatalf("content = %q, want %q", got, body)
		}
	})

	t.Run("formats non-zero expires as RFC3339", func(t *testing.T) {
		client := lifecycleStubClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("ETag", `"5eb63bbbe01eeed093cb22bb8f5acdc3"`)
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			w.Header().Set("Expires", "Wed, 01 Jan 2031 00:00:00 GMT")
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(body))
			}
		})

		d := schema.TestResourceDataRaw(t, dataSourceMinioS3Object().Schema, map[string]interface{}{
			"bucket_name": "test-bucket",
			"object_name": "expiring.txt",
		})
		if diags := dataSourceMinioS3ObjectRead(context.Background(), d, &S3MinioClient{S3Client: client}); diags.HasError() {
			t.Fatalf("dataSourceMinioS3ObjectRead failed: %v", diags)
		}

		if got := d.Get("last_modified").(string); got != "2006-01-02T15:04:05Z" {
			t.Fatalf("last_modified = %q, want %q", got, "2006-01-02T15:04:05Z")
		}
		if got := d.Get("expires").(string); got != "2031-01-01T00:00:00Z" {
			t.Fatalf("expires = %q, want %q", got, "2031-01-01T00:00:00Z")
		}
	})
}

func TestAccDataSourceMinioS3Object_basic(t *testing.T) {
	bucket := "tfacc-object-ds-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceS3ObjectConfig(bucket),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_s3_object.test", "bucket_name", bucket),
					resource.TestCheckResourceAttr("data.minio_s3_object.test", "object_name", "hello.txt"),
					resource.TestCheckResourceAttr("data.minio_s3_object.test", "content", "hello world"),
					resource.TestCheckResourceAttr("data.minio_s3_object.test", "size", "11"),
					resource.TestMatchResourceAttr("data.minio_s3_object.test", "last_modified", regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(Z|[+-]\d{2}:\d{2})$`)),
					resource.TestCheckResourceAttr("data.minio_s3_object.test", "expires", ""),
				),
			},
		},
	})
}

func testAccDataSourceS3ObjectConfig(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket        = %[1]q
  force_destroy = true
}

resource "minio_s3_object" "test" {
  bucket_name = minio_s3_bucket.test.bucket
  object_name = "hello.txt"
  content     = "hello world"
}

data "minio_s3_object" "test" {
  bucket_name = minio_s3_object.test.bucket_name
  object_name = minio_s3_object.test.object_name
}
`, bucket)
}
