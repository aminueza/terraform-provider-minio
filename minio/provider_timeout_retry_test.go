package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccMinioS3Bucket_WithTimeoutRetryConfig(t *testing.T) {
	rInt := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	resourceName := "minio_s3_bucket.bucket"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfigWithTimeoutRetry(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", rInt),
				),
			},
		},
	})
}

func TestAccMinioS3Bucket_WithDefaultTimeoutRetryConfig(t *testing.T) {
	rInt := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	resourceName := "minio_s3_bucket.bucket"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfigWithDefaultTimeoutRetry(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", rInt),
				),
			},
		},
	})
}

func testAccMinioS3BucketConfigWithTimeoutRetry(randInt string) string {
	return fmt.Sprintf(`
provider "minio" {
  request_timeout_seconds = 60
  max_retries             = 10
  retry_delay_ms          = 2000
}

resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl    = "private"
}
`, randInt)
}

func testAccMinioS3BucketConfigWithDefaultTimeoutRetry(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl    = "private"
}
`, randInt)
}
