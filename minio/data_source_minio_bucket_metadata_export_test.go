package minio

import (
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
					resource.TestCheckResourceAttrSet(dataSourceName, "metadata"),
					resource.TestCheckResourceAttr(dataSourceName, "bucket", bucketName),
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
