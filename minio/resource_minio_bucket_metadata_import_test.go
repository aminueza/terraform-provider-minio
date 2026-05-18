package minio

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
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

func TestAccMinioBucketMetadataImport_roundTripPolicy(t *testing.T) {
	bucketName := fmt.Sprintf("tfacc-import-rt-%d", acctest.RandInt())
	resourceName := "minio_bucket_metadata_import.test"
	targetBucket := bucketName

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBucketMetadataImportRoundTripConfig(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "imported_at"),
					resource.TestCheckResourceAttr(resourceName, "bucket", targetBucket),
					testAccCheckMinioBucketHasPolicy(targetBucket),
				),
			},
		},
	})
}

func TestAccMinioBucketMetadataImport_invalidBase64(t *testing.T) {
	bucketName := fmt.Sprintf("tfacc-import-bad-%d", acctest.RandInt())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:      testAccMinioBucketMetadataImportInvalidBase64Config(bucketName),
				ExpectError: regexp.MustCompile(`decoding metadata`),
			},
		},
	})
}

func testAccCheckMinioBucketHasPolicy(bucket string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		client := testAccProvider.Meta().(*S3MinioClient).S3Client
		policy, err := client.GetBucketPolicy(context.Background(), bucket)
		if err != nil {
			return fmt.Errorf("getting policy for target bucket %q: %w", bucket, err)
		}
		if policy == "" {
			return fmt.Errorf("expected target bucket %q to have a policy after import, got empty", bucket)
		}
		return nil
	}
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

func testAccMinioBucketMetadataImportRoundTripConfig(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "source" {
  bucket = "%s-source"
}

resource "minio_s3_bucket_anonymous_access" "source" {
  bucket = minio_s3_bucket.source.bucket
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "AllowRead"
      Effect    = "Allow"
      Principal = { AWS = ["*"] }
      Action    = ["s3:GetObject"]
      Resource  = ["arn:aws:s3:::%s-source/*"]
    }]
  })
}

data "minio_bucket_metadata_export" "source" {
  bucket     = minio_s3_bucket.source.bucket
  depends_on = [minio_s3_bucket_anonymous_access.source]
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

func testAccMinioBucketMetadataImportInvalidBase64Config(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "target" {
  bucket = "%s"
}

resource "minio_bucket_metadata_import" "test" {
  bucket   = minio_s3_bucket.target.bucket
  metadata = "this is not base64!!!"
}
`, bucketName)
}
