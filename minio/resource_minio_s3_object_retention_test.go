package minio

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccMinioS3ObjectRetention_basic(t *testing.T) {
	bucket := "tfacc-ret-" + acctest.RandString(6)
	retainUntil := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	resourceName := "minio_s3_object_retention.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccS3ObjectRetentionConfig(bucket, retainUntil),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "bucket", bucket),
					resource.TestCheckResourceAttr(resourceName, "key", "test-object.txt"),
					resource.TestCheckResourceAttr(resourceName, "mode", "GOVERNANCE"),
					resource.TestCheckResourceAttr(resourceName, "retain_until_date", retainUntil),
				),
			},
		},
	})
}

func TestAccMinioS3ObjectRetention_update(t *testing.T) {
	bucket := "tfacc-ret-upd-" + acctest.RandString(6)
	retainUntil1 := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	retainUntil2 := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	resourceName := "minio_s3_object_retention.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccS3ObjectRetentionConfig(bucket, retainUntil1),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "retain_until_date", retainUntil1),
					resource.TestCheckResourceAttr(resourceName, "mode", "GOVERNANCE"),
				),
			},
			{
				Config: testAccS3ObjectRetentionConfigGovernanceBypass(bucket, retainUntil2),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "retain_until_date", retainUntil2),
					resource.TestCheckResourceAttr(resourceName, "governance_bypass", "true"),
				),
			},
		},
	})
}

func testAccS3ObjectRetentionConfig(bucket, retainUntil string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket         = %[1]q
  object_locking = true
  force_destroy  = true
}

resource "minio_s3_object" "test" {
  bucket_name = minio_s3_bucket.test.id
  object_name = "test-object.txt"
  content     = "hello"
}

resource "minio_s3_object_retention" "test" {
  bucket            = minio_s3_bucket.test.id
  key               = minio_s3_object.test.object_name
  mode              = "GOVERNANCE"
  retain_until_date = %[2]q
}
`, bucket, retainUntil)
}

func testAccS3ObjectRetentionConfigGovernanceBypass(bucket, retainUntil string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket         = %[1]q
  object_locking = true
  force_destroy  = true
}

resource "minio_s3_object" "test" {
  bucket_name = minio_s3_bucket.test.id
  object_name = "test-object.txt"
  content     = "hello"
}

resource "minio_s3_object_retention" "test" {
  bucket            = minio_s3_bucket.test.id
  key               = minio_s3_object.test.object_name
  mode              = "GOVERNANCE"
  retain_until_date = %[2]q
  governance_bypass = true
}
`, bucket, retainUntil)
}

// Retention had no import test: cover a key that contains the "#" separator,
// with and without a version. See issue #1378.
func TestAccMinioS3ObjectRetention_import(t *testing.T) {
	bucket := "tfacc-ret-imp-" + acctest.RandString(6)
	objectKey := "reports#2026.csv"
	retainUntil := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	resourceName := "minio_s3_object_retention.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccS3ObjectRetentionHashKeyConfig(bucket, objectKey, retainUntil),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "key", objectKey),
					resource.TestCheckResourceAttr(resourceName, "id", bucket+"/"+objectKey+"#"),
					resource.TestCheckResourceAttr(resourceName, "mode", "GOVERNANCE"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccImportStateId(resourceName),
			},
			{
				Config: testAccS3ObjectRetentionHashKeyVersionedConfig(bucket, objectKey, retainUntil),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "key", objectKey),
					resource.TestCheckResourceAttrSet(resourceName, "version_id"),
					resource.TestCheckResourceAttr(resourceName, "mode", "GOVERNANCE"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccImportStateId(resourceName),
			},
		},
	})
}

func testAccS3ObjectRetentionHashKeyConfig(bucket, objectKey, retainUntil string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket         = %[1]q
  object_locking = true
  force_destroy  = true
}

resource "minio_s3_object" "test" {
  bucket_name = minio_s3_bucket.test.id
  object_name = %[2]q
  content     = "hello"
}

resource "minio_s3_object_retention" "test" {
  bucket            = minio_s3_bucket.test.id
  key               = minio_s3_object.test.object_name
  mode              = "GOVERNANCE"
  retain_until_date = %[3]q
}
`, bucket, objectKey, retainUntil)
}

func testAccS3ObjectRetentionHashKeyVersionedConfig(bucket, objectKey, retainUntil string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket         = %[1]q
  object_locking = true
  force_destroy  = true
}

resource "minio_s3_object" "test" {
  bucket_name = minio_s3_bucket.test.id
  object_name = %[2]q
  content     = "hello"
}

resource "minio_s3_object_retention" "test" {
  bucket            = minio_s3_bucket.test.id
  key               = minio_s3_object.test.object_name
  version_id        = minio_s3_object.test.version_id
  mode              = "GOVERNANCE"
  retain_until_date = %[3]q
}
`, bucket, objectKey, retainUntil)
}
