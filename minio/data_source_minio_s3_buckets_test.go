package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioS3Buckets_basic(t *testing.T) {
	bucketName := "tfacc-bucket-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceS3BucketsConfig(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_s3_buckets.all", "buckets.#"),
				),
			},
		},
	})
}

func testAccDataSourceS3BucketsConfig(name string) string {
	return `
resource "minio_s3_bucket" "test" {
  bucket = "` + name + `"
}

data "minio_s3_buckets" "all" {
  depends_on = [minio_s3_bucket.test]
}
`
}
