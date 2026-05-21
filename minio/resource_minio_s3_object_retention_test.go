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
