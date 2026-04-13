package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioS3BucketQuota_basic(t *testing.T) {
	bucketName := "tfacc-ds-quota-" + acctest.RandString(8)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioS3BucketQuotaConfig(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_s3_bucket_quota.test", "bucket", bucketName),
					resource.TestCheckResourceAttr("data.minio_s3_bucket_quota.test", "quota", "1048576"),
					resource.TestCheckResourceAttr("data.minio_s3_bucket_quota.test", "type", "hard"),
				),
			},
		},
	})
}

func testAccDataSourceMinioS3BucketQuotaConfig(bucketName string) string {
	return `
resource "minio_s3_bucket" "test" {
  bucket = "` + bucketName + `"
}

resource "minio_s3_bucket_quota" "test" {
  bucket = minio_s3_bucket.test.id
  quota  = 1048576
}

data "minio_s3_bucket_quota" "test" {
  bucket = minio_s3_bucket_quota.test.bucket
}
`
}
