package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioS3Bucket_basic(t *testing.T) {
	bucketName := "tfacc-ds-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_s3_bucket" "test" {
  bucket = "` + bucketName + `"
}

data "minio_s3_bucket" "test" {
  bucket = minio_s3_bucket.test.bucket
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_s3_bucket.test", "bucket", bucketName),
					resource.TestCheckResourceAttrSet("data.minio_s3_bucket.test", "region"),
					resource.TestCheckResourceAttr("data.minio_s3_bucket.test", "versioning_enabled", "false"),
					resource.TestCheckResourceAttr("data.minio_s3_bucket.test", "object_lock_enabled", "false"),
				),
			},
		},
	})
}
