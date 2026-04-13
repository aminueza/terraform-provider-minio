package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioS3BucketPolicy_basic(t *testing.T) {
	bucketName := "tfacc-pol-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_s3_bucket" "test" {
  bucket = "` + bucketName + `"
}

data "minio_s3_bucket_policy" "test" {
  bucket = minio_s3_bucket.test.bucket
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_s3_bucket_policy.test", "bucket", bucketName),
				),
			},
		},
	})
}
