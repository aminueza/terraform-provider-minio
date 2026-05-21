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
// the policy returned by the data source is semantically equivalent to the one the
// resource wrote, exercising the round-trip for the public-read canned type.
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
					// The data source's policy attribute must match the resource's policy attribute.
					resource.TestCheckResourceAttrPair(
						"data.minio_s3_bucket_anonymous_access.test", "policy",
						"minio_s3_bucket_anonymous_access.access", "policy",
					),
					resource.TestCheckResourceAttrPair(
						"data.minio_s3_bucket_anonymous_access.test", "access_type",
						"minio_s3_bucket_anonymous_access.access", "access_type",
					),
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
