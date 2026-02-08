package minio

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/minio-go/v7"
)

func TestAccMinioS3ObjectTags_basic(t *testing.T) {
	bucketName := "tfacc-object-tags-" + acctest.RandString(8)
	objectKey := "test-object.txt"
	resourceName := "minio_s3_object_tags.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioObjectTagsDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3ObjectTagsConfig(bucketName, objectKey),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectTagsExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "key", objectKey),
					resource.TestCheckResourceAttr(resourceName, "tags.Environment", "production"),
					resource.TestCheckResourceAttr(resourceName, "tags.Application", "test"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     fmt.Sprintf("%s/%s", bucketName, objectKey),
			},
			{
				Config: testAccMinioS3ObjectTagsConfigUpdated(bucketName, objectKey),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectTagsExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "tags.Environment", "staging"),
					resource.TestCheckResourceAttr(resourceName, "tags.Application", "updated"),
				),
			},
		},
	})
}

func testAccCheckMinioS3ObjectTagsExists(n string) resource.TestCheckFunc {
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

func testAccCheckMinioObjectTagsDestroy(s *terraform.State) error {
	client := testAccProvider.Meta().(*S3MinioClient)
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_s3_object_tags" {
			continue
		}
		bucket, key, err := parseObjectTagsId(rs.Primary.ID)
		if err != nil {
			return err
		}
		_, err = client.S3Client.GetObjectTagging(ctx, bucket, key, minio.GetObjectTaggingOptions{})
		if err == nil {
			return fmt.Errorf("object tags still exist for %s", rs.Primary.ID)
		}
	}
	return nil
}

func testAccMinioS3ObjectTagsConfig(bucketName, objectKey string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
}

resource "minio_s3_object" "object" {
  bucket_name = minio_s3_bucket.bucket.id
  object_name = "%s"
  content     = "test content"
}

resource "minio_s3_object_tags" "test" {
  bucket = minio_s3_bucket.bucket.id
  key    = minio_s3_object.object.object_name
  tags = {
    Environment = "production"
    Application = "test"
  }
}
`, bucketName, objectKey)
}

func testAccMinioS3ObjectTagsConfigUpdated(bucketName, objectKey string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
}

resource "minio_s3_object" "object" {
  bucket_name = minio_s3_bucket.bucket.id
  object_name = "%s"
  content     = "test content"
}

resource "minio_s3_object_tags" "test" {
  bucket = minio_s3_bucket.bucket.id
  key    = minio_s3_object.object.object_name
  tags = {
    Environment = "staging"
    Application = "updated"
  }
}
`, bucketName, objectKey)
}

func parseObjectTagsId(id string) (bucket, key string, err error) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid object tags ID format: %s", id)
	}
	return parts[0], parts[1], nil
}
