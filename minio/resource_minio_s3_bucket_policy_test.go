package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	awspolicy "github.com/jen20/awspolicyequivalence"
)

func TestAccS3BucketPolicy_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	expectedPolicyText := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
	  "Principal": {"AWS": ["*"]},
      "Resource": [
		"arn:aws:s3:::%[1]s"
      ],
	  "Action": ["s3:ListBucket"]
    }
  ]
}`, name)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketPolicyConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasPolicy("minio_s3_bucket.bucket", expectedPolicyText),
				),
			},
			{
				ResourceName:      "minio_s3_bucket_policy.bucket",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccS3BucketPolicy_policyUpdate(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-test")

	expectedPolicyText1 := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": ["*"]
      },
      "Resource": [
        "arn:aws:s3:::%[1]s"
      ],
	  "Action": ["s3:ListBucket"]
    }
  ]
}`, name)

	expectedPolicyText2 := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": ["*"]
      },
      "Resource": [
        "arn:aws:s3:::%[1]s"
      ],
	  "Action": ["s3:ListBucketVersions"]
    }
  ]
}`, name)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketPolicyConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasPolicy("minio_s3_bucket.bucket", expectedPolicyText1),
				),
			},

			{
				Config: testAccBucketPolicyConfigUpdated(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckBucketHasPolicy("minio_s3_bucket.bucket", expectedPolicyText2),
				),
			},

			{
				ResourceName:      "minio_s3_bucket_policy.bucket",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccBucketPolicyConfig(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
}
resource "minio_s3_bucket_policy" "bucket" {
  bucket = minio_s3_bucket.bucket.bucket
  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
	  "Principal": {"AWS": ["*"]},
      "Resource": [
		"arn:aws:s3:::${minio_s3_bucket.bucket.bucket}"
      ],
	  "Action": ["s3:ListBucket"]
    }
  ]
}
EOF
}
`, bucketName)
}

func testAccBucketPolicyConfigUpdated(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
}
resource "minio_s3_bucket_policy" "bucket" {
  bucket = minio_s3_bucket.bucket.bucket
  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": ["*"]
      },
      "Resource": [
        "arn:aws:s3:::${minio_s3_bucket.bucket.bucket}"
      ],
	  "Action": ["s3:ListBucketVersions"]
    }
  ]
}
EOF
}
`, bucketName)
}

func testAccCheckBucketHasPolicy(n string, expectedPolicyText string) resource.TestCheckFunc {
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
