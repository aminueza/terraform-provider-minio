package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioS3BucketRetention_basic(t *testing.T) {
	bucketName := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioS3BucketRetentionConfig(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_s3_bucket_retention.test", "bucket", bucketName),
					resource.TestCheckResourceAttr("data.minio_s3_bucket_retention.test", "mode", "COMPLIANCE"),
					resource.TestCheckResourceAttr("data.minio_s3_bucket_retention.test", "unit", "DAYS"),
					resource.TestCheckResourceAttr("data.minio_s3_bucket_retention.test", "validity_period", "30"),
				),
			},
		},
	})
}

func testAccDataSourceMinioS3BucketRetentionConfig(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket         = %[1]q
  acl            = "private"
  force_destroy  = true
  object_locking = true
}

resource "minio_s3_bucket_retention" "test" {
  bucket          = minio_s3_bucket.test.bucket
  mode            = "COMPLIANCE"
  unit            = "DAYS"
  validity_period = 30
}

data "minio_s3_bucket_retention" "test" {
  bucket = minio_s3_bucket_retention.test.bucket
}
`, bucketName)
}
