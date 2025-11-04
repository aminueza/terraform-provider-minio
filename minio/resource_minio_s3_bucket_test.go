package minio

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/minio-go/v7"
)

func TestAccMinioS3Bucket_basic(t *testing.T) {
	rInt := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	acl := "public-read"
	resourceName := "minio_s3_bucket.bucket"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfig(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(
						resourceName, "bucket", testAccBucketName(rInt)),
					resource.TestCheckResourceAttr(
						resourceName, "arn", testAccBucketArn(rInt)),
					resource.TestCheckResourceAttr(
						resourceName, "bucket_domain_name", testAccBucketDomainName(rInt)),
					resource.TestCheckResourceAttr(
						resourceName, "acl", testAccBucketACL(acl)),
					resource.TestCheckResourceAttr(
						resourceName, "object_locking", "false"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"force_destroy"},
			},
		},
	})
}

func TestAccMinioS3Bucket_CredentialErrorDoesNotRemoveFromState(t *testing.T) {
    rInt := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
    resourceName := "minio_s3_bucket.bucket"

    endpoint := os.Getenv("MINIO_ENDPOINT")
    user := os.Getenv("MINIO_USER")
    pass := os.Getenv("MINIO_PASSWORD")
    ssl := os.Getenv("MINIO_ENABLE_HTTPS")
    if endpoint == "" || user == "" || pass == "" {
        t.Skip("MINIO_* env vars not set for acceptance test")
    }
    if ssl == "" {
        ssl = "false"
    }

    validConfig := fmt.Sprintf(`
provider "minio" {
  minio_server   = "%s"
  minio_user     = "%s"
  minio_password = "%s"
  minio_ssl      = %s
}

resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl    = "private"
}
`, endpoint, user, pass, ssl, rInt)

    invalidConfig := fmt.Sprintf(`
provider "minio" {
  minio_server   = "%s"
  minio_user     = "%s"
  minio_password = "wrong-password"
  minio_ssl      = %s
}

resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl    = "private"
}
`, endpoint, user, ssl, rInt)

    resource.ParallelTest(t, resource.TestCase{
        PreCheck:          func() { testAccPreCheck(t) },
        ProviderFactories: testAccProviders,
        CheckDestroy:      testAccCheckMinioS3BucketDestroy,
        Steps: []resource.TestStep{
            {
                Config: validConfig,
                Check: resource.ComposeTestCheckFunc(
                    testAccCheckMinioS3BucketExists(resourceName),
                ),
            },
            {
                Config:      invalidConfig,
                ExpectError: regexp.MustCompile(`(?i)(access.?denied|invalid.?access|signature|403)`),
            },
            {
                Config:   validConfig,
                PlanOnly: true,
            },
        },
    })
}

func TestAccMinioS3Bucket_objectLocking(t *testing.T) {
	rInt := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	acl := "public-read"
	resourceName := "minio_s3_bucket.bucket"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfigObjectLockingEnabled(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(
						resourceName, "bucket", testAccBucketName(rInt)),
					resource.TestCheckResourceAttr(
						resourceName, "arn", testAccBucketArn(rInt)),
					resource.TestCheckResourceAttr(
						resourceName, "bucket_domain_name", testAccBucketDomainName(rInt)),
					resource.TestCheckResourceAttr(
						resourceName, "acl", testAccBucketACL(acl)),
					resource.TestCheckResourceAttr(
						resourceName, "object_locking", "true"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"force_destroy"},
			},
		},
	})
}

func TestAccMinioS3Bucket_Bucket_EmptyString(t *testing.T) {
	resourceName := "minio_s3_bucket.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
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
					"force_destroy"},
			},
		},
	})
}

func TestAccMinioS3Bucket_namePrefix(t *testing.T) {
	resourceName := "minio_s3_bucket.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
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
					"force_destroy", "bucket_prefix"},
			},
		},
	})
}

func TestAccMinioS3Bucket_generatedName(t *testing.T) {
	resourceName := "minio_s3_bucket.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
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
					"force_destroy", "bucket_prefix"},
			},
		},
	})
}

func TestAccMinioS3Bucket_UpdateAcl(t *testing.T) {
	ri := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	preConfig := testAccMinioS3BucketConfigWithACL(ri, "public-read")
	postConfig := testAccMinioS3BucketConfigWithACL(ri, "public")
	resourceName := "minio_s3_bucket.bucket"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
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
				ImportStateId:     ri,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"force_destroy"},
			},
			{
				ResourceName: resourceName,
				Config:       postConfig,
				Check:        testAccCheckMinioS3BucketACLInState(resourceName, "public"),
			},
			{
				ResourceName:      resourceName,
				ImportStateId:     ri,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"force_destroy"},
			},
		},
	})
}

func TestAccMinioS3Bucket_UpdateAclToPrivate(t *testing.T) {
	ri := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	preConfig := testAccMinioS3BucketConfigWithACL(ri, "public-read")
	postConfig := testAccMinioS3BucketConfigWithACL(ri, "private")
	resourceName := "minio_s3_bucket.bucket"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: preConfig,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "acl", "public-read"),
				),
			},
			{
				Config: postConfig,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "acl", "private"),
					testAccCheckBucketNotReadableAnonymously(ri),
				),
			},
		},
	})
}

func TestAccMinioS3Bucket_UpdateAclToPrivateIdempotent(t *testing.T) {
	ri := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	config := testAccMinioS3BucketConfigWithACL(ri, "private")
	resourceName := "minio_s3_bucket.bucket"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "acl", "private"),
					testAccCheckBucketNotReadableAnonymously(ri),
				),
			},
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "acl", "private"),
					testAccCheckBucketNotReadableAnonymously(ri),
				),
			},
		},
	})
}

func TestAccMinioS3Bucket_shouldFailNotFound(t *testing.T) {
	rInt := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	resourceName := "minio_s3_bucket.bucket"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
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
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
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

func TestAccMinioS3Bucket_PrivateBucketUnreadable(t *testing.T) {
	ri := fmt.Sprintf("tf-test-bucket-%d", acctest.RandInt())
	preConfig := testAccMinioS3BucketConfigWithACL(ri, "private")
	resourceName := "minio_s3_bucket.bucket"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: preConfig,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(
						resourceName, "acl", "private"),
					testAccCheckBucketNotReadableAnonymously(ri),
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

func testAccCheckMinioS3BucketDestroy(s *terraform.State) (err error) {

	err = providerMinioS3BucketDestroy(testAccProvider.Meta().(*S3MinioClient).S3Client, s)
	if err != nil {
		return
	}

	if testAccSecondProvider.Meta() == nil {
		return
	}

	err = providerMinioS3BucketDestroy(testAccSecondProvider.Meta().(*S3MinioClient).S3Client, s)
	if err != nil {
		return
	}

	if testAccThirdProvider.Meta() == nil {
		return
	}

	err = providerMinioS3BucketDestroy(testAccThirdProvider.Meta().(*S3MinioClient).S3Client, s)
	if err != nil {
		return
	}

	if testAccFourthProvider.Meta() == nil {
		return
	}

	err = providerMinioS3BucketDestroy(testAccFourthProvider.Meta().(*S3MinioClient).S3Client, s)
	return
}

func providerMinioS3BucketDestroy(conn *minio.Client, s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_s3_bucket" {
			continue
		}
		if ok, _ := conn.BucketExists(context.Background(), rs.Primary.ID); ok {
			err := conn.RemoveBucket(context.Background(), rs.Primary.ID)
			if err != nil {
				return fmt.Errorf("error removing bucket: %s", err)
			}

			bucket, err := conn.BucketExists(context.Background(), rs.Primary.ID)
			if err != nil {
				return err
			}
			if !bucket {
				return fmt.Errorf("bucket still exists")
			}
		}
	}
	return nil
}

func testAccCheckMinioS3BucketExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient).S3Client
		isBucket, _ := minioC.BucketExists(context.Background(), rs.Primary.ID)

		if !isBucket {
			return fmt.Errorf("s3 bucket not found")

		}

		return nil

	}
}

func testAccCheckMinioS3DestroyBucket(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no S3 Bucket ID is set")
		}

		conn := testAccProvider.Meta().(*S3MinioClient).S3Client
		err := conn.RemoveBucket(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error destroying Bucket (%s) in testAccCheckMinioS3DestroyBucket: %s", rs.Primary.ID, err)
		}
		return nil
	}
}

func testAccCheckMinioS3BucketACLInState(n string, acl string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		attr, ok := rs.Primary.Attributes["acl"]
		if !ok {
			return fmt.Errorf("attribute acl not found")
		}
		if attr != acl {
			return fmt.Errorf("attribute acl %s, wanted: %s", attr, acl)
		}

		return nil
	}
}

func testAccBucketName(randInt string) string {
	return randInt
}

func testAccBucketArn(randInt string) string {
	return fmt.Sprintf("arn:aws:s3:::%s", randInt)
}

func testAccBucketDomainName(randInt string) string {
	return fmt.Sprintf("http://minio:9000/minio/%s", testAccBucketName(randInt))
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

func testAccMinioS3BucketConfig(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl    = "public-read"
}
`, randInt)
}

func testAccMinioS3BucketConfigObjectLockingEnabled(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl    = "public-read"
  object_locking = true
}
`, randInt)
}

func testAccMinioS3BucketDestroyedConfig(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl    = "public-read"
}
`, randInt)
}

func testAccMinioS3BucketConfigWithACL(name, acl string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
	bucket = "%s"
	acl = "%s"
}
`, name, acl)
}

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

func testAccCheckBucketNotReadableAnonymously(bucket string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resp, err := http.Get("http://" + os.Getenv("MINIO_ENDPOINT") + "/" + bucket)
		if err != nil {
			return err
		}
		if resp.StatusCode != 403 {
			return fmt.Errorf("should not be able to list buckets (Got a %d status)", resp.StatusCode)
		}
		return nil
	}
}
