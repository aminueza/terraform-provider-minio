package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioBucketRetention_basic(t *testing.T) {
	bucketName := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	resourceName := "minio_s3_bucket_retention.retention"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioBucketRetentionDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBucketRetentionConfig_basic(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioBucketRetentionExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "mode", "COMPLIANCE"),
					resource.TestCheckResourceAttr(resourceName, "unit", "DAYS"),
					resource.TestCheckResourceAttr(resourceName, "validity_period", "30"),
				),
			},
		},
	})
}

func TestAccMinioBucketRetention_update(t *testing.T) {
	bucketName := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	resourceName := "minio_s3_bucket_retention.retention"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioBucketRetentionDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBucketRetentionConfig_basic(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioBucketRetentionExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "mode", "COMPLIANCE"),
					resource.TestCheckResourceAttr(resourceName, "unit", "DAYS"),
					resource.TestCheckResourceAttr(resourceName, "validity_period", "30"),
				),
			},
			{
				Config: testAccMinioBucketRetentionConfig_update(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioBucketRetentionExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "mode", "GOVERNANCE"),
					resource.TestCheckResourceAttr(resourceName, "unit", "YEARS"),
					resource.TestCheckResourceAttr(resourceName, "validity_period", "1"),
				),
			},
		},
	})
}

func TestAccMinioBucketRetention_disappears(t *testing.T) {
	bucketName := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	resourceName := "minio_s3_bucket_retention.retention"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioBucketRetentionDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBucketRetentionConfig_basic(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioBucketRetentionExists(resourceName),
					testAccCheckMinioBucketRetentionDisappears(resourceName),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func testAccCheckMinioBucketRetentionExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set")
		}

		client := testAccProvider.Meta().(*S3MinioClient).S3Client
		mode, validity, unit, err := client.GetBucketObjectLockConfig(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error getting bucket retention: %w", err)
		}

		if mode == nil || validity == nil || unit == nil {
			return fmt.Errorf("retention configuration not found")
		}

		return nil
	}
}

func testAccCheckMinioBucketRetentionDestroy(s *terraform.State) error {
	client := testAccProvider.Meta().(*S3MinioClient).S3Client

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_s3_bucket_retention" {
			continue
		}

		// Try to get retention config
		mode, _, _, err := client.GetBucketObjectLockConfig(context.Background(), rs.Primary.ID)
		if err == nil && mode != nil {
			return fmt.Errorf("bucket retention still exists")
		}
	}

	return nil
}

func testAccCheckMinioBucketRetentionDisappears(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		client := testAccProvider.Meta().(*S3MinioClient).S3Client

		// Clear the retention configuration
		err := client.SetBucketObjectLockConfig(context.Background(), rs.Primary.ID, nil, nil, nil)
		if err != nil {
			return fmt.Errorf("error clearing bucket retention: %w", err)
		}

		// Force a read of the configuration to update state
		mode, _, _, err := client.GetBucketObjectLockConfig(context.Background(), rs.Primary.ID)
		if err == nil && mode != nil {
			return fmt.Errorf("bucket retention still exists after clearing")
		}

		return nil
	}
}

func testAccMinioBucketRetentionConfig_basic(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket          = %[1]q
  acl             = "private"
  force_destroy   = true
  object_locking  = true
}

resource "minio_s3_bucket_retention" "retention" {
  bucket          = minio_s3_bucket.test.bucket
  mode            = "COMPLIANCE"
  unit            = "DAYS"
  validity_period = 30

}
`, bucketName)
}

func testAccMinioBucketRetentionConfig_update(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket          = %[1]q
  acl             = "private"
  force_destroy   = true
  object_locking  = true
}

resource "minio_s3_bucket_retention" "retention" {
  bucket          = minio_s3_bucket.test.bucket
  mode            = "GOVERNANCE"
  unit            = "YEARS"
  validity_period = 1

}
`, bucketName)
}
