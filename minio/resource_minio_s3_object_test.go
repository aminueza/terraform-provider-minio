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
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
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
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateId:           fmt.Sprintf("tf-test-bucket-%d/test-object", rInt),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"content", "content_base64", "source", "acl"},
			},
		},
	})
}

func TestAccMinioS3Object_withACL(t *testing.T) {
	rInt := acctest.RandInt()
	resourceName := "minio_s3_object.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
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
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
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
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
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
	conn := testMustGetMinioClient().S3Client

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

		conn := testMustGetMinioClient().S3Client
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

func TestAccMinioS3Object_withMetadata(t *testing.T) {
	rInt := acctest.RandInt()
	resourceName := "minio_s3_object.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		CheckDestroy:      testAccCheckMinioS3ObjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3ObjectConfigWithMetadata(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "metadata.environment", "test"),
					resource.TestCheckResourceAttr(resourceName, "metadata.team", "platform"),
					resource.TestCheckResourceAttr(resourceName, "cache_control", "max-age=3600"),
					resource.TestCheckResourceAttr(resourceName, "content_disposition", "attachment"),
					resource.TestCheckResourceAttr(resourceName, "content_encoding", "identity"),
					resource.TestCheckResourceAttr(resourceName, "storage_class", "STANDARD"),
				),
			},
		},
	})
}

func TestAccMinioS3Object_withMetadataUpdate(t *testing.T) {
	rInt := acctest.RandInt()
	resourceName := "minio_s3_object.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		CheckDestroy:      testAccCheckMinioS3ObjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3ObjectConfigWithMetadata(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "metadata.environment", "test"),
				),
			},
			{
				Config: testAccMinioS3ObjectConfigWithMetadataUpdated(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "metadata.environment", "production"),
					resource.TestCheckResourceAttr(resourceName, "cache_control", "no-cache"),
				),
			},
		},
	})
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

func testAccMinioS3ObjectConfigWithMetadata(rInt int) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = "tf-test-bucket-%d"
  acl    = "public-read-write"
}

resource "minio_s3_object" "test" {
  bucket_name         = minio_s3_bucket.test.bucket
  object_name         = "test-object-meta"
  content             = "test content with metadata"
  content_type        = "text/plain"
  cache_control       = "max-age=3600"
  content_disposition = "attachment"
  content_encoding    = "identity"
  storage_class       = "STANDARD"

  metadata = {
    environment = "test"
    team        = "platform"
  }
}
`, rInt)
}

func testAccMinioS3ObjectConfigWithMetadataUpdated(rInt int) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = "tf-test-bucket-%d"
  acl    = "public-read-write"
}

resource "minio_s3_object" "test" {
  bucket_name         = minio_s3_bucket.test.bucket
  object_name         = "test-object-meta"
  content             = "updated content"
  content_type        = "text/plain"
  cache_control       = "no-cache"
  content_disposition = "attachment"
  content_encoding    = "identity"
  storage_class       = "STANDARD"

  metadata = {
    environment = "production"
    team        = "platform"
  }
}
`, rInt)
}
