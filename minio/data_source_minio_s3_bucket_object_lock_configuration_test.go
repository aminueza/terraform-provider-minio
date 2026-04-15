package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioS3BucketObjectLockConfiguration_basic(t *testing.T) {
	bucketName := fmt.Sprintf("tf-test-objlock-%d", acctest.RandInt())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioS3BucketObjectLockConfigurationConfig(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_s3_bucket_object_lock_configuration.test", "bucket", bucketName),
					resource.TestCheckResourceAttr("data.minio_s3_bucket_object_lock_configuration.test", "object_lock_enabled", "Enabled"),
					resource.TestCheckResourceAttr("data.minio_s3_bucket_object_lock_configuration.test", "rule.0.default_retention.0.mode", "GOVERNANCE"),
					resource.TestCheckResourceAttr("data.minio_s3_bucket_object_lock_configuration.test", "rule.0.default_retention.0.days", "1"),
				),
			},
		},
	})
}

func testAccDataSourceMinioS3BucketObjectLockConfigurationConfig(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket         = %[1]q
  acl            = "private"
  force_destroy  = true
  object_locking = true
}

resource "minio_s3_bucket_object_lock_configuration" "test" {
  bucket = minio_s3_bucket.test.bucket

  rule = [{
    default_retention = [{
      mode = "GOVERNANCE"
      days = 1
    }]
  }]
}

data "minio_s3_bucket_object_lock_configuration" "test" {
  bucket = minio_s3_bucket_object_lock_configuration.test.bucket
}
`, bucketName)
}
