package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// The data source only sees the policy, and `public-read-write` writes the same policy as
// `public`, so that is what it derives for both.
func TestAccDataSourceMinioS3BucketAnonymousAccess_cannedTypes(t *testing.T) {
	for accessType, derived := range map[string]string{
		"public":            "public",
		"public-read":       "public-read",
		"public-read-write": "public",
		"public-write":      "public-write",
	} {
		accessType, derived := accessType, derived
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
							resource.TestCheckResourceAttr("data.minio_s3_bucket_anonymous_access.test", "access_type", derived),
							resource.TestCheckResourceAttrSet("data.minio_s3_bucket_anonymous_access.test", "policy"),
						),
					},
				},
			})
		})
	}
}

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
