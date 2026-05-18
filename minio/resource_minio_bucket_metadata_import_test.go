package minio

import (
	"context"
	"fmt"
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
		CheckDestroy:      testAccCheckMinioBucketMetadataImportDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBucketMetadataImportConfig(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioBucketMetadataImportExists(resourceName),
					resource.TestCheckResourceAttrSet(resourceName, "imported_at"),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
				),
			},
		},
	})
}

func testAccCheckMinioBucketMetadataImportDestroy(s *terraform.State) error {
	conn := testAccProvider.Meta().(*S3MinioClient).S3Client

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_bucket_metadata_import" {
			continue
		}

		exists, err := conn.BucketExists(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("bucket %s still exists", rs.Primary.ID)
		}
	}

	return nil
}

func testAccCheckMinioBucketMetadataImportExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no resource ID is set")
		}

		conn := testAccProvider.Meta().(*S3MinioClient).S3Client

		exists, err := conn.BucketExists(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error checking bucket: %w", err)
		}

		if !exists {
			return fmt.Errorf("bucket %s does not exist", rs.Primary.ID)
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
