package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

func TestAccILMPolicy_basic(t *testing.T) {
	var lifecycleConfig lifecycle.Configuration
	name := fmt.Sprintf("test-ilm-rule-%d", acctest.RandInt())
	resourceName := "minio_ilm_policy.rule"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioILMPolicyConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					resource.TestCheckResourceAttr(resourceName, "bucket", name),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
				),
			},
		},
	})
}

func TestAccILMPolicy_deleteMarkerDays(t *testing.T) {
	var lifecycleConfig lifecycle.Configuration
	name := fmt.Sprintf("test-ilm-rule2-%d", acctest.RandInt())
	resourceName := "minio_ilm_policy.rule2"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioILMPolicyConfigDeleteMarker(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
				),
			},
			{
				Config: testAccMinioILMPolicyConfigDays(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
				),
			},
		},
	})
}

func TestAccILMPolicy_filterTags(t *testing.T) {
	var lifecycleConfig lifecycle.Configuration
	name := fmt.Sprintf("test-ilm-rule3-%d", acctest.RandInt())
	resourceName := "minio_ilm_policy.rule3"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioILMPolicyFilterWithPrefix(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
				),
			},
			{
				Config: testAccMinioILMPolicyFilterWithPrefixAndTags(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioILMPolicyExists(resourceName, &lifecycleConfig),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
				),
			},
		},
	})
}

func testAccCheckMinioLifecycleConfigurationValid(config *lifecycle.Configuration) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if config.Empty() || len(config.Rules) == 0 {
			return fmt.Errorf("lifecycle configuration is empty")
		}
		return nil
	}
}

func testAccCheckMinioILMPolicyExists(n string, config *lifecycle.Configuration) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient).S3Client
		bucketLifecycle, _ := minioC.GetBucketLifecycle(context.Background(), rs.Primary.ID)
		if bucketLifecycle == nil {
			return fmt.Errorf("bucket lifecycle not found")
		}
		*config = *bucketLifecycle

		return nil
	}
}

func testAccMinioILMPolicyConfig(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl    = "public-read"
}
resource "minio_ilm_policy" "rule" {
  bucket = "${minio_s3_bucket.bucket.id}"
  rule {
	id = "asdf"
	expiration = "2022-01-01"
	filter = "temp/"
  }
}
`, randInt)
}

func testAccMinioILMPolicyConfigDays(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket2" {
  bucket = "%s"
  acl    = "public-read"
}
resource "minio_ilm_policy" "rule2" {
  bucket = "${minio_s3_bucket.bucket2.id}"
  rule {
	id = "asdf"
	expiration = "5d"
	filter = "temp/"
  }
}
`, randInt)
}

func testAccMinioILMPolicyConfigDeleteMarker(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket2" {
  bucket = "%s"
  acl    = "public-read"
}
resource "minio_ilm_policy" "rule2" {
  bucket = "${minio_s3_bucket.bucket2.id}"
  rule {
	id = "asdf"
	expiration = "DeleteMarker"
  }
}
`, randInt)
}

func testAccMinioILMPolicyFilterWithPrefix(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket3" {
  bucket = "%s"
  acl    = "public-read"
}
resource "minio_ilm_policy" "rule3" {
  bucket = "${minio_s3_bucket.bucket3.id}"
  rule {
	id = "withPrefix"
	expiration = "5d"
	filter = "temp/"
  }
}
`, randInt)
}

func testAccMinioILMPolicyFilterWithPrefixAndTags(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket3" {
  bucket = "%s"
  acl    = "public-read"
}
resource "minio_ilm_policy" "rule3" {
  bucket = "${minio_s3_bucket.bucket3.id}"
  rule {
	id = "withPrefixAndTags"
	expiration = "5d"
	filter = "temp/"
	tags = {
		key1 = "value1"
		key2 = "value2"
	}
  }
}
`, randInt)
}
