package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioS3BucketTags_basic(t *testing.T) {
	bucketName := "tfacc-bucket-tags-" + acctest.RandString(8)
	resourceName := "minio_s3_bucket_tags.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioBucketTagsDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketTagsConfig(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketTagsExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "tags.Environment", "production"),
					resource.TestCheckResourceAttr(resourceName, "tags.Application", "test"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     bucketName,
			},
			{
				Config: testAccMinioS3BucketTagsConfigUpdated(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketTagsExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "tags.Environment", "staging"),
					resource.TestCheckResourceAttr(resourceName, "tags.Application", "updated"),
				),
			},
		},
	})
}

func testAccCheckMinioS3BucketTagsExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}
		return nil
	}
}

func testAccCheckMinioBucketTagsDestroy(s *terraform.State) error {
	client := testAccProvider.Meta().(*S3MinioClient)
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_s3_bucket_tags" {
			continue
		}
		bucket := rs.Primary.ID
		_, err := client.S3Client.GetBucketTagging(ctx, bucket)
		if err == nil {
			return fmt.Errorf("bucket tags still exist for %s", bucket)
		}
	}
	return nil
}

func testAccMinioS3BucketTagsConfig(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"

  lifecycle {
    ignore_changes = [tags]
  }
}

resource "minio_s3_bucket_tags" "test" {
  bucket = minio_s3_bucket.bucket.id
  tags = {
    Environment = "production"
    Application = "test"
  }
}
`, bucketName)
}

func testAccMinioS3BucketTagsConfigUpdated(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"

  lifecycle {
    ignore_changes = [tags]
  }
}

resource "minio_s3_bucket_tags" "test" {
  bucket = minio_s3_bucket.bucket.id
  tags = {
    Environment = "staging"
    Application = "updated"
  }
}
`, bucketName)
}
