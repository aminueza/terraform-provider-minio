package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioS3BucketVersioning_basic(t *testing.T) {
	bucket := "tfacc-ver-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_s3_bucket" "test" { bucket = "` + bucket + `" }
data "minio_s3_bucket_versioning" "test" { bucket = minio_s3_bucket.test.bucket }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_s3_bucket_versioning.test", "enabled", "false"),
				),
			},
		},
	})
}

func TestAccDataSourceMinioS3BucketEncryption_basic(t *testing.T) {
	bucket := "tfacc-enc-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_s3_bucket" "test" { bucket = "` + bucket + `" }
data "minio_s3_bucket_encryption" "test" { bucket = minio_s3_bucket.test.bucket }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_s3_bucket_encryption.test", "bucket", bucket),
				),
			},
		},
	})
}

func TestAccDataSourceMinioS3BucketCorsConfig_basic(t *testing.T) {
	bucket := "tfacc-cors-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_s3_bucket" "test" { bucket = "` + bucket + `" }
data "minio_s3_bucket_cors_config" "test" { bucket = minio_s3_bucket.test.bucket }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_s3_bucket_cors_config.test", "bucket", bucket),
				),
			},
		},
	})
}

func TestAccDataSourceMinioILMPolicy_basic(t *testing.T) {
	bucket := "tfacc-ilm-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_s3_bucket" "test" { bucket = "` + bucket + `" }
data "minio_ilm_policy" "test" { bucket = minio_s3_bucket.test.bucket }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_ilm_policy.test", "bucket", bucket),
				),
			},
		},
	})
}
