package minio

import (
	"context"
	"fmt"
	"testing"

	awspolicy "github.com/hashicorp/awspolicyequivalence"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccS3BucketAnonymousAccess_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketAnonymousAccessConfig(name, "public-read"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasAnonymousAccess("minio_s3_bucket.bucket", "public-read"),
				),
			},
			{
				ResourceName:      "minio_s3_bucket_anonymous_access.access",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccS3BucketAnonymousAccess_policyOverridesAccessType(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	customPolicy := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {"AWS": ["*"]},
      "Resource": ["arn:aws:s3:::%s/*"],
      "Action": ["s3:GetObject"]
    }
  ]
}`, name)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketAnonymousAccessCustomPolicyWithAccessTypeConfig(name, customPolicy, "public"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasCustomPolicy("minio_s3_bucket.bucket", customPolicy),
				),
			},
		},
	})
}

func TestAccS3BucketAnonymousAccess_update(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketAnonymousAccessConfig(name, "public-read"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasAnonymousAccess("minio_s3_bucket.bucket", "public-read"),
				),
			},
			{
				Config: testAccBucketAnonymousAccessConfig(name, "public-write"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasAnonymousAccess("minio_s3_bucket.bucket", "public-write"),
				),
			},
		},
	})
}

func TestAccS3BucketAnonymousAccess_customPolicy(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	customPolicy := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {"AWS": ["*"]},
      "Resource": ["arn:aws:s3:::%s/*"],
      "Action": ["s3:GetObject"]
    }
  ]
}`, name)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketAnonymousAccessCustomPolicyConfig(name, customPolicy),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasCustomPolicy("minio_s3_bucket.bucket", customPolicy),
				),
			},
		},
	})
}

func testAccBucketAnonymousAccessConfig(bucketName, accessType string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
}

resource "minio_s3_bucket_anonymous_access" "access" {
  bucket = minio_s3_bucket.bucket.id
  access_type = "%s"
}
`, bucketName, accessType)
}

func testAccBucketAnonymousAccessCustomPolicyConfig(bucketName, policy string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
}

resource "minio_s3_bucket_anonymous_access" "access" {
  bucket = minio_s3_bucket.bucket.id
  policy = <<EOF
%s
EOF
}
`, bucketName, policy)
}

func testAccBucketAnonymousAccessCustomPolicyWithAccessTypeConfig(bucketName, policy, accessType string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
}

resource "minio_s3_bucket_anonymous_access" "access" {
  bucket      = minio_s3_bucket.bucket.id
  policy      = <<EOF
%s
EOF
  access_type = "%s"
}
`, bucketName, policy, accessType)
}

func testAccCheckBucketHasAnonymousAccess(n string, accessType string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient).S3Client
		actualPolicyText, err := minioC.GetBucketPolicy(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error on GetBucketPolicy: %v", err)
		}

		// Generate expected policy for the access type
		bucket := &S3MinioBucket{MinioBucket: rs.Primary.ID}
		var (
			expectedPolicyText string
			expectedPolicyErr  error
		)

		switch accessType {
		case "public":
			expectedPolicyText, expectedPolicyErr = marshalPolicy(PublicPolicy(bucket))
		case "public-read":
			expectedPolicyText, expectedPolicyErr = marshalPolicy(ReadOnlyPolicy(bucket))
		case "public-read-write":
			expectedPolicyText, expectedPolicyErr = marshalPolicy(ReadWritePolicy(bucket))
		case "public-write":
			expectedPolicyText, expectedPolicyErr = marshalPolicy(WriteOnlyPolicy(bucket))
		default:
			return fmt.Errorf("unknown access type: %s", accessType)
		}
		if expectedPolicyErr != nil {
			return fmt.Errorf("error marshaling expected policy for %s: %w", accessType, expectedPolicyErr)
		}

		equivalent, err := awspolicy.PoliciesAreEquivalent(actualPolicyText, expectedPolicyText)
		if err != nil {
			return fmt.Errorf("error testing policy equivalence: %s", err)
		}
		if !equivalent {
			return fmt.Errorf("non-equivalent policy error:\n\nexpected: %s\n\ngot: %s",
				expectedPolicyText, actualPolicyText)
		}

		return nil
	}
}

func testAccCheckBucketHasCustomPolicy(n string, expectedPolicyText string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient).S3Client
		actualPolicyText, err := minioC.GetBucketPolicy(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error on GetBucketPolicy: %v", err)
		}

		equivalent, err := awspolicy.PoliciesAreEquivalent(actualPolicyText, expectedPolicyText)
		if err != nil {
			return fmt.Errorf("error testing policy equivalence: %s", err)
		}
		if !equivalent {
			return fmt.Errorf("non-equivalent policy error:\n\nexpected: %s\n\ngot: %s",
				expectedPolicyText, actualPolicyText)
		}

		return nil
	}
}
