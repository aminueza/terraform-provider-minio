package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
					resource.TestCheckResourceAttr(resourceName, "id", fmt.Sprintf("tf-test-bucket-%d/test-object", rInt)),
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
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"content", "content_base64", "source", "acl"},
			},
		},
	})
}

func TestAccMinioS3Object_nestedKey(t *testing.T) {
	rInt := acctest.RandInt()
	resourceName := "minio_s3_object.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3ObjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3ObjectConfigWithName(rInt, "dir/file.txt"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "id", fmt.Sprintf("tf-test-bucket-%d/dir/file.txt", rInt)),
					resource.TestCheckResourceAttr(resourceName, "bucket_name", fmt.Sprintf("tf-test-bucket-%d", rInt)),
					resource.TestCheckResourceAttr(resourceName, "object_name", "dir/file.txt"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"content", "content_base64", "source", "acl"},
			},
		},
	})
}

func TestAccMinioS3Object_upgradeFromOldID(t *testing.T) {
	rInt := acctest.RandInt()
	resourceName := "minio_s3_object.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		CheckDestroy: testAccCheckMinioS3ObjectDestroy,
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"minio": {
						Source:            "aminueza/minio",
						VersionConstraint: "3.46.4",
					},
				},
				Config: testAccMinioS3ObjectConfigBasic(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "id", "test-object"),
				),
			},
			{
				ProviderFactories:  testAccProviders,
				Config:             testAccMinioS3ObjectConfigBasic(rInt),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				ProviderFactories: testAccProviders,
				Config:            testAccMinioS3ObjectConfigBasic(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "id", fmt.Sprintf("tf-test-bucket-%d/test-object", rInt)),
				),
			},
		},
	})
}

// Overwriting the object outside Terraform must show up as drift and be
// reverted by the next apply. See issue #1330.
func TestAccMinioS3Object_contentChangedOutsideTerraform(t *testing.T) {
	rInt := acctest.RandInt()
	resourceName := "minio_s3_object.test"
	bucketName := fmt.Sprintf("tf-test-bucket-%d", rInt)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3ObjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3ObjectConfigBasic(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectExists(resourceName),
				),
			},
			{
				PreConfig: func() {
					testAccOverwriteObjectOutsideTerraform(t, bucketName, "test-object", "externally overwritten content")
				},
				Config:             testAccMinioS3ObjectConfigBasic(rInt),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccMinioS3ObjectConfigBasic(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectExists(resourceName),
					testAccCheckMinioS3ObjectContent(bucketName, "test-object", "test content"),
				),
			},
		},
	})
}

func testAccOverwriteObjectOutsideTerraform(t *testing.T, bucket, object, content string) {
	t.Helper()
	data := []byte(content)
	// Pass the exact size: an unknown-size upload is stored as a streamed
	// multipart upload whose ETag carries a -1 suffix, which the drift
	// detection skips. A known-size upload gets the bare content MD5, like
	// mc cp of a file.
	_, err := testAccClient().S3Client.PutObject(context.Background(), bucket, object, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{})
	if err != nil {
		t.Fatalf("overwriting object %s/%s outside Terraform: %v", bucket, object, err)
	}
}

// testAccCheckMinioS3ObjectContent reads the object straight from the server
// and compares it against an expected literal, independently of the provider.
func testAccCheckMinioS3ObjectContent(bucket, object, expected string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		conn := testAccClient().S3Client
		obj, err := conn.GetObject(context.Background(), bucket, object, minio.GetObjectOptions{})
		if err != nil {
			return fmt.Errorf("reading object %s/%s: %w", bucket, object, err)
		}
		defer obj.Close()
		data, err := io.ReadAll(obj)
		if err != nil {
			return fmt.Errorf("reading object %s/%s: %w", bucket, object, err)
		}
		if string(data) != expected {
			return fmt.Errorf("object %s/%s content is %q, expected %q", bucket, object, string(data), expected)
		}
		return nil
	}
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
	conn := testAccClient().S3Client

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

		conn := testAccClient().S3Client
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
	return testAccMinioS3ObjectConfigWithName(rInt, "test-object")
}

func testAccMinioS3ObjectConfigWithName(rInt int, objectName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = "tf-test-bucket-%d"
}

resource "minio_s3_object" "test" {
  bucket_name = minio_s3_bucket.test.bucket
  object_name = %q
  content     = "test content"
}
`, rInt, objectName)
}

func testAccMinioS3ObjectConfigWithACL(rInt int, acl string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = "tf-test-bucket-%d"
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
		ProviderFactories: testAccProviders,
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
		ProviderFactories: testAccProviders,
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
