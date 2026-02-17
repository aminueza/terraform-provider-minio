package minio

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func testAccPreCheckSSE(t *testing.T) {
	if os.Getenv("MINIO_KMS_CONFIGURED") == "" {
		t.Skip("Skipping SSE tests: MINIO_KMS_CONFIGURED not set. Server-side encryption requires a KMS backend (e.g., KES) to be configured.")
	}
}

func TestAccMinioBucketServerSideEncryption_sseS3(t *testing.T) {
	bucketName := "tfacc-sse-s3-" + acctest.RandString(8)
	resourceName := "minio_s3_bucket_server_side_encryption.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSSE(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioBucketEncryptionDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBucketEncryptionSSES3(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioBucketEncryptionExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "encryption_type", "AES256"),
					resource.TestCheckResourceAttr(resourceName, "kms_key_id", ""),
				),
			},
		},
	})
}

func TestAccMinioBucketServerSideEncryption_sseKMS(t *testing.T) {
	bucketName := "tfacc-sse-kms-" + acctest.RandString(8)
	resourceName := "minio_s3_bucket_server_side_encryption.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSSE(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioBucketEncryptionDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBucketEncryptionSSEKMS(bucketName, "my-minio-key"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioBucketEncryptionExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "encryption_type", "aws:kms"),
					resource.TestCheckResourceAttr(resourceName, "kms_key_id", "my-minio-key"),
				),
			},
		},
	})
}

func TestAccMinioBucketServerSideEncryption_update(t *testing.T) {
	bucketName := "tfacc-sse-upd-" + acctest.RandString(8)
	resourceName := "minio_s3_bucket_server_side_encryption.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSSE(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioBucketEncryptionDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBucketEncryptionSSES3(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioBucketEncryptionExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "encryption_type", "AES256"),
				),
			},
			{
				Config: testAccMinioBucketEncryptionSSEKMS(bucketName, "my-minio-key"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioBucketEncryptionExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "encryption_type", "aws:kms"),
					resource.TestCheckResourceAttr(resourceName, "kms_key_id", "my-minio-key"),
				),
			},
		},
	})
}

func TestAccMinioBucketServerSideEncryption_import(t *testing.T) {
	bucketName := "tfacc-sse-imp-" + acctest.RandString(8)
	resourceName := "minio_s3_bucket_server_side_encryption.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSSE(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioBucketEncryptionDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBucketEncryptionSSES3(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioBucketEncryptionExists(resourceName),
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

func TestAccMinioBucketServerSideEncryption_updateKMStoS3(t *testing.T) {
	bucketName := "tfacc-sse-k2s-" + acctest.RandString(8)
	resourceName := "minio_s3_bucket_server_side_encryption.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSSE(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioBucketEncryptionDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBucketEncryptionSSEKMS(bucketName, "my-minio-key"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioBucketEncryptionExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "encryption_type", "aws:kms"),
					resource.TestCheckResourceAttr(resourceName, "kms_key_id", "my-minio-key"),
				),
			},
			{
				Config: testAccMinioBucketEncryptionSSES3(bucketName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioBucketEncryptionExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "encryption_type", "AES256"),
					resource.TestCheckResourceAttr(resourceName, "kms_key_id", ""),
				),
			},
		},
	})
}

func TestAccMinioBucketServerSideEncryption_importKMS(t *testing.T) {
	bucketName := "tfacc-sse-ik-" + acctest.RandString(8)
	resourceName := "minio_s3_bucket_server_side_encryption.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSSE(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioBucketEncryptionDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBucketEncryptionSSEKMS(bucketName, "my-minio-key"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioBucketEncryptionExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "encryption_type", "aws:kms"),
					resource.TestCheckResourceAttr(resourceName, "kms_key_id", "my-minio-key"),
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

func TestAccMinioBucketServerSideEncryption_invalidType(t *testing.T) {
	bucketName := "tfacc-sse-inv-" + acctest.RandString(8)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSSE(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioBucketEncryptionDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  provider = kmsminio
  bucket   = "%s"
}

resource "minio_s3_bucket_server_side_encryption" "test" {
  provider        = kmsminio
  bucket          = minio_s3_bucket.bucket.id
  encryption_type = "invalid"
}
`, bucketName),
				ExpectError: regexp.MustCompile(`expected encryption_type to be one of \["aws:kms" "AES256"\]`),
			},
		},
	})
}

func TestAccMinioBucketServerSideEncryption_kmsWithoutKeyID(t *testing.T) {
	bucketName := "tfacc-sse-nk-" + acctest.RandString(8)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccPreCheckSSE(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioBucketEncryptionDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  provider = kmsminio
  bucket   = "%s"
}

resource "minio_s3_bucket_server_side_encryption" "test" {
  provider        = kmsminio
  bucket          = minio_s3_bucket.bucket.id
  encryption_type = "aws:kms"
}
`, bucketName),
				ExpectError: regexp.MustCompile(`kms_key_id is required when encryption_type is "aws:kms"`),
			},
		},
	})
}

func testAccCheckMinioBucketEncryptionExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		client := testAccKmsProvider.Meta().(*S3MinioClient).S3Client
		_, err := client.GetBucketEncryption(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error getting bucket encryption for %s: %v", rs.Primary.ID, err)
		}

		return nil
	}
}

func testAccCheckMinioBucketEncryptionDestroy(s *terraform.State) error {
	client := testAccKmsProvider.Meta().(*S3MinioClient).S3Client
	ctx := context.Background()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_s3_bucket_server_side_encryption" {
			continue
		}

		config, err := client.GetBucketEncryption(ctx, rs.Primary.ID)
		if err != nil {
			continue
		}

		if len(config.Rules) > 0 {
			return fmt.Errorf("bucket encryption still exists for %s", rs.Primary.ID)
		}
	}

	return nil
}

func testAccMinioBucketEncryptionSSES3(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  provider = kmsminio
  bucket   = "%s"
}

resource "minio_s3_bucket_server_side_encryption" "test" {
  provider        = kmsminio
  bucket          = minio_s3_bucket.bucket.id
  encryption_type = "AES256"
}
`, bucketName)
}

func testAccMinioBucketEncryptionSSEKMS(bucketName, keyID string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  provider = kmsminio
  bucket   = "%s"
}

resource "minio_s3_bucket_server_side_encryption" "test" {
  provider        = kmsminio
  bucket          = minio_s3_bucket.bucket.id
  encryption_type = "aws:kms"
  kms_key_id      = "%s"
}
`, bucketName, keyID)
}
