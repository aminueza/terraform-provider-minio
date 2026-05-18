package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccMinioBucketMetadataImport_basic(t *testing.T) {
	bucketName := fmt.Sprintf("tfacc-import-%d", acctest.RandInt())
	resourceName := "minio_bucket_metadata_import.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBucketMetadataImportConfig(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "imported_at"),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
				),
			},
		},
	})
}

func TestAccMinioBucketMetadataImport_withMetadata(t *testing.T) {
	bucketName := fmt.Sprintf("tfacc-import-meta-%d", acctest.RandInt())
	resourceName := "minio_bucket_metadata_import.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBucketMetadataImportWithMetadataConfig(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "imported_at"),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
				),
			},
		},
	})
}

func testAccMinioBucketMetadataImportConfig(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "source" {
  bucket = "%s-source"
}

data "minio_bucket_metadata_export" "source" {
  bucket = minio_s3_bucket.source.bucket
}

resource "minio_s3_bucket" "target" {
  bucket = "%s"
}

resource "minio_bucket_metadata_import" "test" {
  bucket   = minio_s3_bucket.target.bucket
  metadata = data.minio_bucket_metadata_export.source.metadata
}
`, bucketName, bucketName)
}

func testAccMinioBucketMetadataImportWithMetadataConfig(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "source" {
  bucket = "%s-source"
}

resource "minio_s3_bucket_policy" "source" {
  bucket = minio_s3_bucket.source.bucket
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "ReadAccess"
        Action    = "s3:GetObject"
        Resource  = "arn:aws:s3:::%s-source/*"
        Effect    = "Allow"
        Principal = "*"
      }
    ]
  })
}

resource "minio_s3_bucket_tags" "source" {
  bucket = minio_s3_bucket.source.bucket
  tags = {
    environment = "test"
    managed_by  = "terraform"
  }
}

data "minio_bucket_metadata_export" "source" {
  bucket     = minio_s3_bucket.source.bucket
  depends_on = [minio_s3_bucket_policy.source, minio_s3_bucket_tags.source]
}

resource "minio_s3_bucket" "target" {
  bucket = "%s"
}

resource "minio_bucket_metadata_import" "test" {
  bucket   = minio_s3_bucket.target.bucket
  metadata = data.minio_bucket_metadata_export.source.metadata
}
`, bucketName, bucketName, bucketName)
}
