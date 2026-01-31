package minio

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func testAccPreCheckCORS(t *testing.T) {
	if os.Getenv("MINIO_CORS_ENABLED") == "" {
		t.Skip("Skipping CORS tests: MINIO_CORS_ENABLED not set. Bucket CORS requires MinIO Enterprise/AIStor subscription.")
	}
}

func TestAccS3BucketCors_basic(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_s3_bucket_cors.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckCORS(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketCorsDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketCorsConfig_basic(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketCorsExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.allowed_methods.#", "2"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.allowed_methods.0", "GET"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.allowed_methods.1", "PUT"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.allowed_origins.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.allowed_origins.0", "https://example.com"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.max_age_seconds", "3000"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccS3BucketCors_multipleRules(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_s3_bucket_cors.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckCORS(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketCorsDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketCorsConfig_multipleRules(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketCorsExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.#", "2"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.id", "rule1"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.allowed_methods.#", "3"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.allowed_origins.#", "2"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.1.id", "rule2"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.1.allowed_methods.#", "2"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccS3BucketCors_update(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_s3_bucket_cors.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckCORS(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketCorsDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketCorsConfig_basic(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketCorsExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.allowed_methods.#", "2"),
				),
			},
			{
				Config: testAccBucketCorsConfig_updated(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketCorsExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.allowed_methods.#", "4"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.allowed_headers.#", "2"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.expose_headers.#", "1"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccS3BucketCors_allHeaders(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_s3_bucket_cors.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckCORS(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketCorsDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketCorsConfig_allHeaders(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketCorsExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.allowed_headers.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.allowed_headers.0", "*"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.allowed_origins.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "cors_rule.0.allowed_origins.0", "*"),
				),
			},
		},
	})
}

func TestAccS3BucketCors_import(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_s3_bucket_cors.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckCORS(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketCorsDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketCorsConfig_multipleRules(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketCorsExists(resourceName),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckMinioS3BucketCorsExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient).S3Client
		corsConfig, err := minioC.GetBucketCors(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error getting bucket CORS configuration: %v", err)
		}

		if corsConfig == nil || len(corsConfig.CORSRules) == 0 {
			return fmt.Errorf("CORS configuration not found for bucket: %s", rs.Primary.ID)
		}

		return nil
	}
}

func testAccCheckMinioS3BucketCorsDestroy(s *terraform.State) error {
	minioC := testAccProvider.Meta().(*S3MinioClient).S3Client

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_s3_bucket_cors" {
			continue
		}

		corsConfig, err := minioC.GetBucketCors(context.Background(), rs.Primary.ID)
		if err != nil {
			errMsg := err.Error()
			if errMsg == "The CORS configuration does not exist" || errMsg == "NoSuchCORSConfiguration: The CORS configuration does not exist" {
				continue
			}
			return fmt.Errorf("error getting bucket CORS configuration: %v", err)
		}

		if corsConfig != nil && len(corsConfig.CORSRules) > 0 {
			return fmt.Errorf("CORS configuration still exists for bucket: %s", rs.Primary.ID)
		}
	}

	return nil
}

func testAccBucketCorsConfig_basic(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = %[1]q
}

resource "minio_s3_bucket_cors" "test" {
  bucket = minio_s3_bucket.test.bucket

  cors_rule {
    allowed_methods = ["GET", "PUT"]
    allowed_origins = ["https://example.com"]
    max_age_seconds = 3000
  }
}
`, bucketName)
}

func testAccBucketCorsConfig_multipleRules(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = %[1]q
}

resource "minio_s3_bucket_cors" "test" {
  bucket = minio_s3_bucket.test.bucket

  cors_rule {
    id              = "rule1"
    allowed_methods = ["GET", "PUT", "POST"]
    allowed_origins = ["https://example.com", "https://www.example.com"]
    allowed_headers = ["*"]
    expose_headers  = ["ETag"]
    max_age_seconds = 3600
  }

  cors_rule {
    id              = "rule2"
    allowed_methods = ["GET", "HEAD"]
    allowed_origins = ["https://mobile.example.com"]
    max_age_seconds = 1800
  }
}
`, bucketName)
}

func testAccBucketCorsConfig_updated(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = %[1]q
}

resource "minio_s3_bucket_cors" "test" {
  bucket = minio_s3_bucket.test.bucket

  cors_rule {
    allowed_methods = ["GET", "PUT", "POST", "DELETE"]
    allowed_origins = ["https://example.com"]
    allowed_headers = ["Content-Type", "Authorization"]
    expose_headers  = ["ETag"]
    max_age_seconds = 7200
  }
}
`, bucketName)
}

func testAccBucketCorsConfig_allHeaders(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = %[1]q
}

resource "minio_s3_bucket_cors" "test" {
  bucket = minio_s3_bucket.test.bucket

  cors_rule {
    allowed_methods = ["GET", "PUT", "POST", "DELETE", "HEAD"]
    allowed_origins = ["*"]
    allowed_headers = ["*"]
    max_age_seconds = 3600
  }
}
`, bucketName)
}
