package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	awspolicy "github.com/hashicorp/awspolicyequivalence"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/minio-go/v7/pkg/policy"
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

func TestAccS3BucketAnonymousAccess_public(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketAnonymousAccessConfig(name, "public"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckAnonymousAccessGrantsDataAccessOnly("minio_s3_bucket_anonymous_access.access"),
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

// public-read-write and public are the same policy, so a read that classified by shape alone
// would rename the configured access_type on every refresh and leave a plan that never settles.
func TestAccS3BucketAnonymousAccess_readWriteStaysStable(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketAnonymousAccessConfig(name, "public-read-write"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasAnonymousAccess("minio_s3_bucket.bucket", "public-read-write"),
					resource.TestCheckResourceAttr("minio_s3_bucket_anonymous_access.access", "access_type", "public-read-write"),
				),
			},
			{
				// Same configuration again: the step fails on a non-empty plan.
				Config: testAccBucketAnonymousAccessConfig(name, "public-read-write"),
				Check:  resource.TestCheckResourceAttr("minio_s3_bucket_anonymous_access.access", "access_type", "public-read-write"),
			},
		},
	})
}

// Reads the policy back off the server and asserts what a MinIO client makes of it, rather than
// comparing it to the builder that wrote it.
func testAccCheckAnonymousAccessGrantsDataAccessOnly(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		bucket := decodeAnonymousAccessID(rs.Primary.ID)
		minioC := testAccProvider.Meta().(*S3MinioClient).S3Client
		actualPolicyText, err := minioC.GetBucketPolicy(context.Background(), bucket)
		if err != nil {
			return fmt.Errorf("error on GetBucketPolicy: %v", err)
		}

		var parsed policy.BucketAccessPolicy
		if err := json.Unmarshal([]byte(actualPolicyText), &parsed); err != nil {
			return fmt.Errorf("policy on server is not valid JSON: %v (%s)", err, actualPolicyText)
		}

		if got := policy.GetPolicy(parsed.Statements, bucket, ""); got != policy.BucketPolicyReadWrite {
			return fmt.Errorf("expected the server policy to classify as %q, got %q (mc would report this as `custom`): %s",
				policy.BucketPolicyReadWrite, got, actualPolicyText)
		}

		for _, statement := range parsed.Statements {
			for _, action := range []string{
				"s3:CreateBucket",
				"s3:DeleteBucket",
				"s3:DeleteBucketPolicy",
				"s3:PutBucketPolicy",
				"s3:GetBucketPolicy",
				"s3:GetBucketNotification",
				"s3:PutBucketNotification",
				"s3:ListenBucketNotification",
			} {
				if statement.Actions.Contains(action) {
					return fmt.Errorf("server policy grants %s to anonymous callers: %s", action, actualPolicyText)
				}
			}
		}

		return nil
	}
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

// testAccCheckBucketHasAnonymousAccess asserts what a MinIO client makes of the policy on the
// server: policy.GetPolicy is the same function mc runs to answer `mc anonymous get`, so a shape
// mc would call `custom` fails here. Comparing against the builder that wrote the policy would
// pass no matter which shape the builder produces.
func testAccCheckBucketHasAnonymousAccess(n string, accessType string) resource.TestCheckFunc {
	expectedLabels := map[string]policy.BucketPolicy{
		"public-read":       policy.BucketPolicyReadOnly,
		"public-write":      policy.BucketPolicyWriteOnly,
		"public-read-write": policy.BucketPolicyReadWrite,
		"public":            policy.BucketPolicyReadWrite,
	}

	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		expected, ok := expectedLabels[accessType]
		if !ok {
			return fmt.Errorf("unknown access type: %s", accessType)
		}

		minioC := testAccProvider.Meta().(*S3MinioClient).S3Client
		actualPolicyText, err := minioC.GetBucketPolicy(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error on GetBucketPolicy: %v", err)
		}

		var parsed policy.BucketAccessPolicy
		if err := json.Unmarshal([]byte(actualPolicyText), &parsed); err != nil {
			return fmt.Errorf("policy on server is not valid JSON: %v (%s)", err, actualPolicyText)
		}

		if got := policy.GetPolicy(parsed.Statements, rs.Primary.ID, ""); got != expected {
			return fmt.Errorf("expected %s to classify as %q, got %q (mc would report this as `custom`): %s",
				accessType, expected, got, actualPolicyText)
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
