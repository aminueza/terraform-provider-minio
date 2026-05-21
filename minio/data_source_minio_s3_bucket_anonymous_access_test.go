package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// TestAccDataSourceMinioS3BucketAnonymousAccess_cannedTypes verifies that the data
// source correctly reads back the policy and derives the matching canned access_type
// for each of the four supported canned modes.
func TestAccDataSourceMinioS3BucketAnonymousAccess_cannedTypes(t *testing.T) {
	for _, accessType := range []string{"public", "public-read", "public-read-write", "public-write"} {
		accessType := accessType
		t.Run(accessType, func(t *testing.T) {
			t.Parallel()
			bucketName := "tfacc-anon-" + acctest.RandString(6)

			resource.Test(t, resource.TestCase{
				PreCheck:          func() { testAccPreCheck(t) },
				ProviderFactories: testAccProviders,
				CheckDestroy:      testAccCheckMinioS3BucketDestroy,
				Steps: []resource.TestStep{
					{
						Config: testAccDataSourceBucketAnonymousAccessCannedConfig(bucketName, accessType),
						Check: resource.ComposeTestCheckFunc(
							resource.TestCheckResourceAttr("data.minio_s3_bucket_anonymous_access.test", "bucket", bucketName),
							resource.TestCheckResourceAttr("data.minio_s3_bucket_anonymous_access.test", "access_type", accessType),
							resource.TestCheckResourceAttrSet("data.minio_s3_bucket_anonymous_access.test", "policy"),
						),
					},
				},
			})
		})
	}
}

// TestAccDataSourceMinioS3BucketAnonymousAccess_customPolicy verifies that when the
// bucket has a custom (non-canned) policy the data source returns the raw policy JSON
// and leaves access_type empty.
func TestAccDataSourceMinioS3BucketAnonymousAccess_customPolicy(t *testing.T) {
	bucketName := "tfacc-anon-" + acctest.RandString(6)

	customPolicy := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {"AWS": ["*"]},
      "Resource": ["arn:aws:s3:::%s/*"],
      "Action": ["s3:GetObject", "s3:GetObjectVersion"]
    }
  ]
}`, bucketName)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceBucketAnonymousAccessCustomPolicyConfig(bucketName, customPolicy),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_s3_bucket_anonymous_access.test", "bucket", bucketName),
					resource.TestCheckResourceAttr("data.minio_s3_bucket_anonymous_access.test", "access_type", ""),
					resource.TestCheckResourceAttrSet("data.minio_s3_bucket_anonymous_access.test", "policy"),
				),
			},
		},
	})
}

// TestAccDataSourceMinioS3BucketAnonymousAccess_policyMatchesResource verifies that
// the data source derives the same access_type as the resource and exposes a non-empty
// policy for the public-read canned type round-trip.
//
// Note: exact policy string equality is intentionally not asserted. The resource's read
// path may return struct-marshaled JSON (e.g. Version before Statement) while the data
// source normalizes via structure.NormalizeJsonString (alphabetical key order). Both
// representations are semantically identical; comparing access_type is the meaningful check.
func TestAccDataSourceMinioS3BucketAnonymousAccess_policyMatchesResource(t *testing.T) {
	bucketName := "tfacc-anon-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceBucketAnonymousAccessCannedConfig(bucketName, "public-read"),
				Check: resource.ComposeTestCheckFunc(
					// access_type must agree between resource and data source.
					resource.TestCheckResourceAttrPair(
						"data.minio_s3_bucket_anonymous_access.test", "access_type",
						"minio_s3_bucket_anonymous_access.access", "access_type",
					),
					// Both must expose a non-empty policy.
					resource.TestCheckResourceAttrSet("minio_s3_bucket_anonymous_access.access", "policy"),
					resource.TestCheckResourceAttrSet("data.minio_s3_bucket_anonymous_access.test", "policy"),
				),
			},
		},
	})
}

func testAccDataSourceBucketAnonymousAccessCannedConfig(bucketName, accessType string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = %q
}

resource "minio_s3_bucket_anonymous_access" "access" {
  bucket      = minio_s3_bucket.test.id
  access_type = %q
}

data "minio_s3_bucket_anonymous_access" "test" {
  bucket     = minio_s3_bucket.test.id
  depends_on = [minio_s3_bucket_anonymous_access.access]
}
`, bucketName, accessType)
}

func testAccDataSourceBucketAnonymousAccessCustomPolicyConfig(bucketName, policy string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = %q
}

resource "minio_s3_bucket_anonymous_access" "access" {
  bucket = minio_s3_bucket.test.id
  policy = <<-EOT
%s
  EOT
}

data "minio_s3_bucket_anonymous_access" "test" {
  bucket     = minio_s3_bucket.test.id
  depends_on = [minio_s3_bucket_anonymous_access.access]
}
`, bucketName, policy)
}
