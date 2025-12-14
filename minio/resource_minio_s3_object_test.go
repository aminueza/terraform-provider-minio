package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/minio-go/v7"
)

func TestAccMinioS3Object_basic(t *testing.T) {
	rInt := acctest.RandInt()
	resourceName := "minio_s3_object.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3ObjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3ObjectConfigBasic(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket_name", fmt.Sprintf("tf-test-bucket-%d", rInt)),
					resource.TestCheckResourceAttr(resourceName, "object_name", "test-object"),
					resource.TestCheckResourceAttr(resourceName, "content", "test content"),
					resource.TestCheckResourceAttr(resourceName, "acl", "private"),
					resource.TestCheckResourceAttrSet(resourceName, "etag"),
				),
			},
		},
	})
}

func TestAccMinioS3Object_withACL(t *testing.T) {
	rInt := acctest.RandInt()
	resourceName := "minio_s3_object.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3ObjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3ObjectConfigWithACL(rInt, "public-read"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "acl", "public-read"),
				),
			},
		},
	})
}

func TestAccMinioS3Object_withACLUpdate(t *testing.T) {
	rInt := acctest.RandInt()
	resourceName := "minio_s3_object.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3ObjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3ObjectConfigWithACL(rInt, "private"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "acl", "private"),
				),
			},
			{
				Config: testAccMinioS3ObjectConfigWithACL(rInt, "public-read"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "acl", "public-read"),
				),
			},
		},
	})
}

func TestAccMinioS3Object_withContentType(t *testing.T) {
	rInt := acctest.RandInt()
	resourceName := "minio_s3_object.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3ObjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3ObjectConfigWithContentType(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "content_type", "text/plain"),
				),
			},
		},
	})
}

func testAccCheckMinioS3ObjectDestroy(s *terraform.State) error {
	conn := testAccProvider.Meta().(*S3MinioClient).S3Client

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_s3_object" {
			continue
		}

		bucketName := rs.Primary.Attributes["bucket_name"]
		objectName := rs.Primary.Attributes["object_name"]

		_, err := conn.StatObject(context.Background(), bucketName, objectName, minio.StatObjectOptions{})
		if err == nil {
			return fmt.Errorf("object %s still exists in bucket %s", objectName, bucketName)
		}
	}

	return nil
}

func testAccCheckMinioS3ObjectExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		conn := testAccProvider.Meta().(*S3MinioClient).S3Client
		bucketName := rs.Primary.Attributes["bucket_name"]
		objectName := rs.Primary.Attributes["object_name"]

		_, err := conn.StatObject(context.Background(), bucketName, objectName, minio.StatObjectOptions{})
		if err != nil {
			return fmt.Errorf("object %s does not exist in bucket %s: %s", objectName, bucketName, err)
		}

		return nil
	}
}

func testAccMinioS3ObjectConfigBasic(rInt int) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = "tf-test-bucket-%d"
  acl    = "public-read-write"
}

resource "minio_s3_object" "test" {
  bucket_name = minio_s3_bucket.test.bucket
  object_name = "test-object"
  content     = "test content"
}
`, rInt)
}

func testAccMinioS3ObjectConfigWithACL(rInt int, acl string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = "tf-test-bucket-%d"
  acl    = "public-read-write"
}

resource "minio_s3_object" "test" {
  bucket_name = minio_s3_bucket.test.bucket
  object_name = "test-object"
  content     = "test content"
  acl         = "%s"
}
`, rInt, acl)
}

func testAccMinioS3ObjectConfigWithContentType(rInt int) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = "tf-test-bucket-%d"
  acl    = "public-read-write"
}

resource "minio_s3_object" "test" {
  bucket_name  = minio_s3_bucket.test.bucket
  object_name  = "test-object.txt"
  content      = "test content"
  content_type = "text/plain"
}
`, rInt)
}
