package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccS3BucketVersioning_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketVersioningConfig(name, "Enabled"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasVersioning("minio_s3_bucket_versioning.bucket", "Enabled"),
				),
			},
			{
				ResourceName:      "minio_s3_bucket_versioning.bucket",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccS3BucketVersioning_update(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketVersioningConfig(name, "Enabled"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasVersioning("minio_s3_bucket_versioning.bucket", "Enabled"),
				),
			},
			{
				Config: testAccBucketVersioningConfig(name, "Suspended"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasVersioning("minio_s3_bucket_versioning.bucket", "Suspended"),
				),
			},
			{
				ResourceName:      "minio_s3_bucket_versioning.bucket",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccBucketVersioningConfig(bucketName string, status string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
}

resource "minio_s3_bucket_versioning" "bucket" {
  bucket = minio_s3_bucket.bucket.bucket
  versioning_configuration {
    status = "%s"
  }
}
`, bucketName, status)
}

func testAccCheckBucketHasVersioning(n string, expectedStatus string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient).S3Client
		actualConfig, err := minioC.GetBucketVersioning(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error on GetBucketVersioning: %v", err)
		}

		if actualConfig.Status != expectedStatus {
			return fmt.Errorf("non-equivalent status error:\n\nexpected: %s\n\ngot: %s", expectedStatus, actualConfig.Status)
		}

		return nil
	}
}
