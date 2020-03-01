package minio

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform/helper/acctest"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
)

func TestAccMinioS3Bucket_basic(t *testing.T) {
	rInt := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	acl := "public-read"
	resourceName := "minio_s3_bucket.bucket"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfig(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(
						resourceName, "bucket", testAccBucketName(rInt)),
					resource.TestCheckResourceAttr(
						resourceName, "bucket_domain_name", testAccBucketDomainName(rInt)),
					resource.TestCheckResourceAttr(
						resourceName, "acl", testAccBucketACL(acl)),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"force_destroy", "acl"},
			},
		},
	})
}

func TestAccMinioS3Bucket_Bucket_EmptyString(t *testing.T) {
	resourceName := "minio_s3_bucket.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfigBucketEmptyString,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestMatchResourceAttr(resourceName, "bucket", regexp.MustCompile("^terraform-")),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"force_destroy", "acl"},
			},
		},
	})
}

func TestAccMinioS3Bucket_namePrefix(t *testing.T) {
	resourceName := "minio_s3_bucket.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfigNamePrefix,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestMatchResourceAttr(
						resourceName, "bucket", regexp.MustCompile("^tf-test-")),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"force_destroy", "acl", "bucket_prefix"},
			},
		},
	})
}

func TestAccMinioS3Bucket_generatedName(t *testing.T) {
	resourceName := "minio_s3_bucket.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfigGeneratedName,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"force_destroy", "acl", "bucket_prefix"},
			},
		},
	})
}

func TestAccMinioS3Bucket_UpdateAcl(t *testing.T) {
	ri := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	preConfig := fmt.Sprintf(testAccMinioS3BucketConfigWithACL, ri)
	postConfig := fmt.Sprintf(testAccMinioS3BucketConfigWithACLUpdate, ri)
	resourceName := "minio_s3_bucket.bucket"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: preConfig,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(
						resourceName, "acl", "public-read"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"force_destroy", "acl"},
			},
			{
				Config: postConfig,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(
						resourceName, "acl", "private"),
				),
			},
		},
	})
}

func TestAccMinioS3Bucket_shouldFailNotFound(t *testing.T) {
	rInt := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	resourceName := "minio_s3_bucket.bucket"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketDestroyedConfig(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					testAccCheckMinioS3DestroyBucket(resourceName),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccMinioS3Bucket_forceDestroy(t *testing.T) {
	resourceName := "minio_s3_bucket.bucket"
	rInt := acctest.RandInt()
	bucketName := fmt.Sprintf("tf-test-bucket-%d", rInt)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfigForceDestroy(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
				),
			},
		},
	})
}

func TestMinioS3BucketName(t *testing.T) {
	validDNSNames := []string{
		"foobar",
		"foo.bar",
		"foo.bar.baz",
		"1234",
		"foo-bar",
		strings.Repeat("x", 63),
	}

	for _, v := range validDNSNames {
		if err := validateS3BucketName(v); err != nil {
			t.Fatalf("%q should be a valid S3 bucket name", v)
		}
	}

	invalidDNSNames := []string{
		"foo..bar",
		"Foo.Bar",
		"192.168.0.1",
		"127.0.0.1",
		".foo",
		"bar.",
		"foo_bar",
		strings.Repeat("x", 64),
	}

	for _, v := range invalidDNSNames {
		if err := validateS3BucketName(v); err == nil {
			t.Fatalf("%q should not be a valid S3 bucket name", v)
		}
	}
}

func testAccCheckMinioS3BucketDestroy(s *terraform.State) error {
	conn := testAccProvider.Meta().(*S3MinioClient).S3Client

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_s3_bucket" {
			continue
		}
		if ok, _ := conn.BucketExists(rs.Primary.ID); ok {
			err := conn.RemoveBucket(rs.Primary.ID)
			if err != nil {
				return fmt.Errorf("Error removing bucket: %s", err)
			}

			bucket, err := conn.BucketExists(rs.Primary.ID)
			if !bucket {
				return fmt.Errorf("Bucket still exists")
			}
		}
	}
	return nil
}

func testAccCheckMinioS3BucketExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient).S3Client
		isBucket, _ := minioC.BucketExists(rs.Primary.ID)

		if !isBucket {
			return fmt.Errorf("S3 bucket not found")

		}

		return nil

	}
}

func testAccCheckMinioS3DestroyBucket(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No S3 Bucket ID is set")
		}

		conn := testAccProvider.Meta().(*S3MinioClient).S3Client
		err := conn.RemoveBucket(rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("Error destroying Bucket (%s) in testAccCheckMinioS3DestroyBucket: %s", rs.Primary.ID, err)
		}
		return nil
	}
}

func testAccBucketName(randInt string) string {
	return fmt.Sprintf("%s", randInt)
}

func testAccBucketDomainName(randInt string) string {
	return fmt.Sprintf("http://localhost:9000/minio/%s", randInt)
}

func testAccBucketACL(acl string) string {
	validAcls := map[string]string{
		"private":           "private",
		"public-write":      "public-write",
		"public-read":       "public-read",
		"public-read-write": "public-read-write",
		"public":            "public",
	}

	policyACL, policyExists := validAcls[acl]

	if policyExists {
		return policyACL
	}
	return ""
}

func testAccMinioS3BucketPolicy(randInt int, partition string) string {
	return fmt.Sprintf(`{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Sid": "",
			"Effect": "Allow",
			"Principal": {"Minio": "*"},
			"Action": "s3:GetObject",
			"Resource": "arn:%s:s3:::tf-test-bucket-%d/*"
		}
	]
}
`, partition, randInt)
}

func testAccMinioS3BucketConfig(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl    = "public-read"
}
`, randInt)
}

func testAccMinioS3BucketConfigWithNoTags(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = %[1]q
  acl = "private"
  force_destroy = false
}
`, bucketName)
}

func testAccMinioS3BucketDestroyedConfig(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl    = "public-read"
}
`, randInt)
}

func testAccMinioS3BucketConfigWithEmptyPolicy(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl    = "public-read"
  policy = ""
}
`, randInt)
}

var testAccMinioS3BucketConfigWithACL = `
resource "minio_s3_bucket" "bucket" {
	bucket = "%s"
	acl = "public-read"
}
`

var testAccMinioS3BucketConfigWithACLUpdate = `
resource "minio_s3_bucket" "bucket" {
	bucket = "%s"
	acl = "private"
}
`

func testAccMinioS3BucketConfigForceDestroy(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl = "private"
  force_destroy = true
}
`, bucketName)
}

const testAccMinioS3BucketConfigBucketEmptyString = `
resource "minio_s3_bucket" "test" {
  acl = "private"
  bucket = ""
}
`

const testAccMinioS3BucketConfigNamePrefix = `
resource "minio_s3_bucket" "test" {
	acl = "private"
	bucket_prefix = "tf-test-"
}
`

const testAccMinioS3BucketConfigGeneratedName = `
resource "minio_s3_bucket" "test" {
	acl = "private"
	bucket_prefix = "tf-test-"
}
`
