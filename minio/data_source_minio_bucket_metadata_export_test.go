package minio

import (
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccDataSourceMinioBucketMetadataExport_basic(t *testing.T) {
	bucketName := fmt.Sprintf("tfacc-export-%d", acctest.RandInt())
	dataSourceName := "data.minio_bucket_metadata_export.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioBucketMetadataExportConfig(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckDataSourceMinioBucketMetadataExportExists(dataSourceName),
					resource.TestCheckResourceAttr(dataSourceName, "bucket", bucketName),
					testAccCheckDataSourceMinioBucketMetadataExportValidZip(dataSourceName),
				),
			},
		},
	})
}

func testAccCheckDataSourceMinioBucketMetadataExportExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no data source ID is set")
		}

		return nil
	}
}

func testAccCheckDataSourceMinioBucketMetadataExportValidZip(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		metadata := rs.Primary.Attributes["metadata"]
		if metadata == "" {
			return fmt.Errorf("metadata attribute is empty")
		}

		decoded, err := base64.StdEncoding.DecodeString(metadata)
		if err != nil {
			return fmt.Errorf("metadata is not valid base64: %w", err)
		}

		if len(decoded) < 4 {
			return fmt.Errorf("decoded metadata too short to be a valid zip file")
		}

		if decoded[0] != 0x50 || decoded[1] != 0x4B || decoded[2] != 0x03 || decoded[3] != 0x04 {
			return fmt.Errorf("decoded metadata does not start with zip magic bytes (PK\\x03\\x04)")
		}

		return nil
	}
}

func testAccDataSourceMinioBucketMetadataExportConfig(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = "%s"
}

data "minio_bucket_metadata_export" "test" {
  bucket = minio_s3_bucket.test.bucket
}
`, bucketName)
}
