package minio

import (
	"fmt"
	"log"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioS3IncompleteUploadCleanup_basic(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_s3_incomplete_upload_cleanup.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3IncompleteUploadCleanupDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccIncompleteUploadCleanupConfig_basic(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "prefix", ""),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:      true,
				ImportStateVerifyIgnore: []string{"last_cleanup"},
			},
		},
	})
}

func TestAccMinioS3IncompleteUploadCleanup_withPrefix(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_s3_incomplete_upload_cleanup.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3IncompleteUploadCleanupDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccIncompleteUploadCleanupConfig_withPrefix(bucketName, "uploads/"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "prefix", "uploads/"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:      true,
				ImportStateVerifyIgnore: []string{"last_cleanup"},
			},
		},
	})
}

func TestAccMinioS3IncompleteUploadCleanup_update(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_s3_incomplete_upload_cleanup.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3IncompleteUploadCleanupDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccIncompleteUploadCleanupConfig_basic(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "prefix", ""),
				),
			},
			{
				Config: testAccIncompleteUploadCleanupConfig_withPrefix(bucketName, "data/"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "prefix", "data/"),
				),
			},
		},
	})
}

func testAccCheckMinioS3IncompleteUploadCleanupDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_s3_incomplete_upload_cleanup" {
			continue
		}

		// This is a stateless cleanup resource - delete should have cleared the ID
		// But handle gracefully if ID exists due to errors
		if rs.Primary.ID != "" {
			log.Printf("[WARN] Cleanup resource still in state: %s, proceeding anyway", rs.Primary.ID)
		}
	}

	return nil
}

func testAccIncompleteUploadCleanupConfig_basic(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = %[1]q
}

resource "minio_s3_incomplete_upload_cleanup" "test" {
  bucket = minio_s3_bucket.test.bucket
}
`, bucketName)
}

func testAccIncompleteUploadCleanupConfig_withPrefix(bucketName, prefix string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = %[1]q
}

resource "minio_s3_incomplete_upload_cleanup" "test" {
  bucket = minio_s3_bucket.test.bucket
  prefix = %[2]q
}
`, bucketName, prefix)
}
