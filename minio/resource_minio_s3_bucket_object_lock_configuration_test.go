package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccMinioS3BucketObjectLockConfiguration_basic(t *testing.T) {
	bucketName := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketObjectLockConfiguration_basic(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_s3_bucket_object_lock_configuration.test", "bucket", bucketName),
					resource.TestCheckResourceAttr("minio_s3_bucket_object_lock_configuration.test", "object_lock_enabled", "Enabled"),
					resource.TestCheckResourceAttr("minio_s3_bucket_object_lock_configuration.test", "rule.#", "1"),
					resource.TestCheckResourceAttr("minio_s3_bucket_object_lock_configuration.test", "rule.0.default_retention.#", "1"),
					resource.TestCheckResourceAttr("minio_s3_bucket_object_lock_configuration.test", "rule.0.default_retention.0.mode", "GOVERNANCE"),
					resource.TestCheckResourceAttr("minio_s3_bucket_object_lock_configuration.test", "rule.0.default_retention.0.days", "30"),
				),
			},
			{
				ResourceName:      "minio_s3_bucket_object_lock_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccMinioS3BucketObjectLockConfiguration_years(t *testing.T) {
	bucketName := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketObjectLockConfiguration_years(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_s3_bucket_object_lock_configuration.test", "bucket", bucketName),
					resource.TestCheckResourceAttr("minio_s3_bucket_object_lock_configuration.test", "rule.0.default_retention.0.mode", "COMPLIANCE"),
					resource.TestCheckResourceAttr("minio_s3_bucket_object_lock_configuration.test", "rule.0.default_retention.0.years", "7"),
				),
			},
		},
	})
}

func TestAccMinioS3BucketObjectLockConfiguration_update(t *testing.T) {
	bucketName := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketObjectLockConfiguration_basic(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_s3_bucket_object_lock_configuration.test", "rule.0.default_retention.0.mode", "GOVERNANCE"),
					resource.TestCheckResourceAttr("minio_s3_bucket_object_lock_configuration.test", "rule.0.default_retention.0.days", "30"),
				),
			},
			{
				Config: testAccMinioS3BucketObjectLockConfiguration_updated(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_s3_bucket_object_lock_configuration.test", "rule.0.default_retention.0.mode", "COMPLIANCE"),
					resource.TestCheckResourceAttr("minio_s3_bucket_object_lock_configuration.test", "rule.0.default_retention.0.days", "90"),
				),
			},
		},
	})
}

func testAccMinioS3BucketObjectLockConfiguration_basic(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket         = %[1]q
  object_locking = true
}

resource "minio_s3_bucket_object_lock_configuration" "test" {
  bucket              = minio_s3_bucket.test.bucket
  object_lock_enabled = "Enabled"

  rule {
    default_retention {
      mode = "GOVERNANCE"
      days = 30
    }
  }
}
`, bucketName)
}

func testAccMinioS3BucketObjectLockConfiguration_years(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket         = %[1]q
  object_locking = true
}

resource "minio_s3_bucket_object_lock_configuration" "test" {
  bucket = minio_s3_bucket.test.bucket

  rule {
    default_retention {
      mode  = "COMPLIANCE"
      years = 7
    }
  }
}
`, bucketName)
}

func testAccMinioS3BucketObjectLockConfiguration_updated(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket         = %[1]q
  object_locking = true
}

resource "minio_s3_bucket_object_lock_configuration" "test" {
  bucket = minio_s3_bucket.test.bucket

  rule {
    default_retention {
      mode = "COMPLIANCE"
      days = 90
    }
  }
}
`, bucketName)
}
