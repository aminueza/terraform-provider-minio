package minio

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

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

func TestAccMinioS3Bucket_migrateBucketToBucketPrefix_incompatibleForcesReplacement(t *testing.T) {
	bucketName := fmt.Sprintf("tf-migrate-incompat-%d", acctest.RandInt())
	prefix := fmt.Sprintf("tf-mig-inc-%s-", acctest.RandString(10))
	resourceName := "minio_s3_bucket.test"

	var originalBucketName string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfigWithBucket(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					testAccCaptureBucketName(resourceName, &originalBucketName),
				),
			},
			{
				Config: testAccMinioS3BucketConfigWithBucketPrefix(prefix),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					testAccCheckBucketNameHasPrefix(resourceName, prefix),
					testAccCheckBucketNameDiffers(resourceName, &originalBucketName),
				),
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

func TestAccMinioS3Bucket_migrateBucketToBucketPrefix(t *testing.T) {
	prefix := fmt.Sprintf("tf-migrate-%d-", acctest.RandInt())
	bucketName := prefix + "existing"
	resourceName := "minio_s3_bucket.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfigWithBucket(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
				),
			},
			{
				Config: testAccMinioS3BucketConfigWithBucketPrefix(prefix),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
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

func TestAccMinioS3Bucket_migrateBucketToBucketPrefix_fromExactBucketName(t *testing.T) {
	bucketName := fmt.Sprintf("tf-migrate-base-%d", acctest.RandInt())
	prefix := bucketName + "-"
	resourceName := "minio_s3_bucket.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfigWithBucket(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
				),
			},
			{
				Config: testAccMinioS3BucketConfigWithBucketPrefix(prefix),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
				),
			},
		},
	})
}

func TestAccMinioS3Bucket_migrateBucketPrefixToBucket(t *testing.T) {
	prefix := fmt.Sprintf("tf-migrate-rev-%d-", acctest.RandInt())
	resourceName := "minio_s3_bucket.test"

	var bucketName string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfigWithBucketPrefix(prefix),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					testAccCaptureBucketName(resourceName, &bucketName),
				),
			},
			{
				PreConfig: func() {
				},
				Config: testAccMinioS3BucketConfigWithBucketDynamic(&bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					testAccCheckBucketNameMatches(resourceName, &bucketName),
				),
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
					resource.TestCheckResourceAttr(resourceName, "force_destroy", "true"),
					testAccCheckMinioS3BucketAddObjects(resourceName, "test-object-1", "test-object-2"),
				),
			},
		},
	})
}

func TestAccMinioS3Bucket_forceDestroyEmpty(t *testing.T) {
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
					resource.TestCheckResourceAttr(resourceName, "force_destroy", "true"),
				),
			},
		},
	})
}

func TestAccMinioS3Bucket_forceDestroyWithManyObjects(t *testing.T) {
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
					testAccCheckMinioS3BucketAddManyObjects(resourceName, 50),
				),
			},
		},
	})
}

func TestAccMinioS3Bucket_forceDestroyFalseWithObjects(t *testing.T) {
	resourceName := "minio_s3_bucket.bucket"
	rInt := acctest.RandInt()
	bucketName := fmt.Sprintf("tf-test-bucket-%d", rInt)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfigNoForceDestroy(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "force_destroy", "false"),
					testAccCheckMinioS3BucketAddObjects(resourceName, "test-object-1"),
				),
			},
			{
				Config:      testAccMinioS3BucketConfigNoForceDestroy(bucketName),
				Destroy:     true,
				ExpectError: regexp.MustCompile(`bucket .* is not empty`),
			},
			{
				// Clean up: switch to force_destroy=true so the bucket can be deleted
				Config: testAccMinioS3BucketConfigForceDestroy(bucketName),
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

		maxRetries := 6
		for i := 0; i < maxRetries; i++ {
			isBucket, err := minioC.BucketExists(context.Background(), rs.Primary.ID)
			if err != nil {
				return fmt.Errorf("error checking bucket existence: %s", err)
			}

			if isBucket {
				return nil
			}

			if i < maxRetries-1 {
				time.Sleep(time.Duration((i+1)*200) * time.Millisecond)
			}
		}

		return fmt.Errorf("s3 bucket not found after %d attempts", maxRetries)
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
	return fmt.Sprintf("http://minio:9000/%s", testAccBucketName(randInt))
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

func testAccMinioS3BucketConfigNoForceDestroy(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl = "private"
  force_destroy = false
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
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+os.Getenv("MINIO_ENDPOINT")+"/"+bucket, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 403 {
			return fmt.Errorf("should not be able to list buckets (Got a %d status)", resp.StatusCode)
		}
		return nil
	}
}

func testAccCheckMinioS3BucketAddObjects(resourceName string, objects ...string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		client := testAccProvider.Meta().(*S3MinioClient).S3Client
		bucketName := rs.Primary.ID

		for _, obj := range objects {
			_, err := client.PutObject(
				context.Background(),
				bucketName,
				obj,
				strings.NewReader("test content"),
				int64(len("test content")),
				minio.PutObjectOptions{},
			)
			if err != nil {
				return fmt.Errorf("error adding object %s: %s", obj, err)
			}
		}

		return nil
	}
}

func testAccCheckMinioS3BucketAddManyObjects(resourceName string, count int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		client := testAccProvider.Meta().(*S3MinioClient).S3Client
		bucketName := rs.Primary.ID

		for i := 0; i < count; i++ {
			objName := fmt.Sprintf("test-object-%d", i)
			_, err := client.PutObject(
				context.Background(),
				bucketName,
				objName,
				strings.NewReader("test content"),
				int64(len("test content")),
				minio.PutObjectOptions{},
			)
			if err != nil {
				return fmt.Errorf("error adding object %s: %s", objName, err)
			}
		}

		return nil
	}
}

func TestAccMinioS3Bucket_tags(t *testing.T) {
	rInt := acctest.RandInt()
	resourceName := "minio_s3_bucket.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketConfigWithTags(rInt, map[string]string{
					"Environment": "test",
					"Project":     "terraform",
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "tags.Environment", "test"),
					resource.TestCheckResourceAttr(resourceName, "tags.Project", "terraform"),
				),
			},
			{
				Config: testAccMinioS3BucketConfigWithTags(rInt, map[string]string{
					"Environment": "production",
					"Team":        "platform",
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "tags.Environment", "production"),
					resource.TestCheckResourceAttr(resourceName, "tags.Team", "platform"),
					resource.TestCheckNoResourceAttr(resourceName, "tags.Project"),
				),
			},
			{
				Config: testAccMinioS3BucketConfigBasic(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists(resourceName),
					testAccCheckMinioS3BucketTagsRemoved(resourceName),
					resource.TestCheckResourceAttr(resourceName, "tags.%", "0"),
				),
			},
		},
	})
}

func testAccMinioS3BucketConfigBasic(rInt int) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
	bucket = "test-bucket-%d"
}
`, rInt)
}

func testAccMinioS3BucketConfigWithTags(rInt int, tags map[string]string) string {
	tagsStr := ""
	for k, v := range tags {
		tagsStr += fmt.Sprintf("    %s = \"%s\"\n", k, v)
	}

	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
	bucket = "test-bucket-%d"

	tags = {
%s  }
}
`, rInt, tagsStr)
}

func testAccCheckMinioS3BucketTagsRemoved(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient).S3Client
		tags, err := minioC.GetBucketTagging(context.Background(), rs.Primary.ID)
		if err != nil {
			var minioErr minio.ErrorResponse
			if errors.As(err, &minioErr) && minioErr.Code == "NoSuchTagSet" {
				return nil
			}
			return fmt.Errorf("error reading bucket tags: %s", err)
		}

		if tags != nil && tags.Count() > 0 {
			return fmt.Errorf("expected bucket to have no tags, but got %d tags", tags.Count())
		}

		return nil
	}
}

func TestIsNoSuchBucketError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name: "NoSuchBucket error code",
			err: minio.ErrorResponse{
				Code:       "NoSuchBucket",
				Message:    "The specified bucket does not exist",
				StatusCode: http.StatusNotFound,
			},
			expected: true,
		},
		{
			name: "404 status code without NoSuchBucket code",
			err: minio.ErrorResponse{
				Code:       "",
				Message:    "Not Found",
				StatusCode: http.StatusNotFound,
			},
			expected: true,
		},
		{
			name:     "string error containing NoSuchBucket",
			err:      fmt.Errorf("The bucket NoSuchBucket error occurred"),
			expected: true,
		},
		{
			name:     "string error containing does not exist",
			err:      fmt.Errorf("The specified bucket does not exist"),
			expected: true,
		},
		{
			name: "AccessDenied error",
			err: minio.ErrorResponse{
				Code:       "AccessDenied",
				Message:    "Access Denied",
				StatusCode: http.StatusForbidden,
			},
			expected: false,
		},
		{
			name:     "generic error",
			err:      fmt.Errorf("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNoSuchBucketError(tt.err)
			if result != tt.expected {
				t.Errorf("isNoSuchBucketError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func testAccMinioS3BucketConfigWithBucket(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = "%s"
  acl    = "private"
}
`, bucketName)
}

func testAccMinioS3BucketConfigWithBucketPrefix(prefix string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket_prefix = "%s"
  acl           = "private"
}
`, prefix)
}

func testAccMinioS3BucketConfigWithBucketDynamic(bucketName *string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = "%s"
  acl    = "private"
}
`, *bucketName)
}

func testAccCaptureBucketName(resourceName string, bucketName *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}
		*bucketName = rs.Primary.ID
		return nil
	}
}

func testAccCheckBucketNameMatches(resourceName string, expectedName *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}
		if rs.Primary.ID != *expectedName {
			return fmt.Errorf("bucket name mismatch: got %s, expected %s", rs.Primary.ID, *expectedName)
		}
		return nil
	}
}

func testAccCheckBucketNameDiffers(resourceName string, unexpectedName *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}
		if rs.Primary.ID == *unexpectedName {
			return fmt.Errorf("bucket name unexpectedly unchanged: %s", rs.Primary.ID)
		}
		return nil
	}
}

func testAccCheckBucketNameHasPrefix(resourceName string, prefix string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}
		if !strings.HasPrefix(rs.Primary.ID, prefix) {
			return fmt.Errorf("bucket name %s does not have expected prefix %s", rs.Primary.ID, prefix)
		}
		return nil
	}
}
