package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioS3Objects_basic(t *testing.T) {
	bucket := "tfacc-objects-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceS3ObjectsConfig(bucket),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_s3_objects.test", "keys.#", "2"),
					resource.TestCheckResourceAttr("data.minio_s3_objects.test", "keys.0", "dir/file1.txt"),
					resource.TestCheckResourceAttr("data.minio_s3_objects.test", "keys.1", "dir/file2.txt"),
				),
			},
		},
	})
}

func testAccDataSourceS3ObjectsConfig(bucket string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = %[1]q
}

resource "minio_s3_object" "file1" {
  bucket_name = minio_s3_bucket.test.bucket
  object_name = "dir/file1.txt"
  content     = "hello"
}

resource "minio_s3_object" "file2" {
  bucket_name = minio_s3_bucket.test.bucket
  object_name = "dir/file2.txt"
  content     = "world"
}

data "minio_s3_objects" "test" {
  bucket = minio_s3_bucket.test.bucket
  prefix = "dir/"

  depends_on = [minio_s3_object.file1, minio_s3_object.file2]
}
`, bucket)
}
