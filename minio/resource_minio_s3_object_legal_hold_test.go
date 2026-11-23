package minio

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/minio-go/v7"
)

func TestAccMinioS3ObjectLegalHold_basic(t *testing.T) {
	bucketName := "tfacc-legal-hold-" + acctest.RandString(8)
	objectKey := "test-object.txt"
	resourceName := "minio_s3_object_legal_hold.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioObjectLegalHoldDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3ObjectLegalHoldConfig(bucketName, objectKey, "ON"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectLegalHoldExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "key", objectKey),
					resource.TestCheckResourceAttr(resourceName, "status", "ON"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     fmt.Sprintf("%s/%s", bucketName, objectKey),
			},
			{
				Config: testAccMinioS3ObjectLegalHoldConfig(bucketName, objectKey, "OFF"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectLegalHoldExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "status", "OFF"),
				),
			},
		},
	})
}

func TestAccMinioS3ObjectLegalHold_off(t *testing.T) {
	bucketName := "tfacc-legal-hold-" + acctest.RandString(8)
	objectKey := "test-object-off.txt"
	resourceName := "minio_s3_object_legal_hold.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioObjectLegalHoldDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3ObjectLegalHoldConfig(bucketName, objectKey, "OFF"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectLegalHoldExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "key", objectKey),
					resource.TestCheckResourceAttr(resourceName, "status", "OFF"),
				),
			},
		},
	})
}

func testAccCheckMinioS3ObjectLegalHoldExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}
		return nil
	}
}

func testAccCheckMinioObjectLegalHoldDestroy(s *terraform.State) error {
	client := testAccClient()
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_s3_object_legal_hold" {
			continue
		}
		bucket, key, versionID, err := parseObjectVersionID(rs.Primary.ID)
		if err != nil {
			return err
		}
		status, err := client.S3Client.GetObjectLegalHold(ctx, bucket, key, minio.GetObjectLegalHoldOptions{VersionID: versionID})
		if err != nil {
			var minioErr minio.ErrorResponse
			if errors.As(err, &minioErr) && (objectLockReadGone(minioErr) || minioErr.Code == "NoSuchObjectLockConfiguration") {
				continue
			}
			return fmt.Errorf("error checking legal hold for %s: %s", rs.Primary.ID, err)
		}
		if status != nil && *status == minio.LegalHoldEnabled {
			return fmt.Errorf("legal hold still enabled for %s", rs.Primary.ID)
		}
	}
	return nil
}

func testAccMinioS3ObjectLegalHoldConfig(bucketName, objectKey, status string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket         = "%s"
  object_locking = true
  force_destroy  = true
}

resource "minio_s3_object" "object" {
  bucket_name = minio_s3_bucket.bucket.id
  object_name = "%s"
  content     = "test content"
}

resource "minio_s3_object_legal_hold" "test" {
  bucket = minio_s3_bucket.bucket.id
  key    = minio_s3_object.object.object_name
  status = "%s"
}
`, bucketName, objectKey, status)
}

// A key that contains the "#" separator has to survive the round trip through
// the stored ID, including when it has no version. See issue #1378.
func TestAccMinioS3ObjectLegalHold_importKeyWithHash(t *testing.T) {
	bucketName := "tfacc-legal-hold-" + acctest.RandString(8)
	objectKey := "reports#2026.csv"
	resourceName := "minio_s3_object_legal_hold.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioObjectLegalHoldDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3ObjectLegalHoldConfig(bucketName, objectKey, "ON"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectLegalHoldExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "key", objectKey),
					resource.TestCheckResourceAttr(resourceName, "status", "ON"),
					resource.TestCheckResourceAttr(resourceName, "id", bucketName+"/"+objectKey+"#"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccImportStateId(resourceName),
			},
		},
	})
}

// The same key, held on a specific version: the ID carries the key's own "#" and
// the version separator. See issue #1378.
func TestAccMinioS3ObjectLegalHold_importKeyWithHashVersioned(t *testing.T) {
	bucketName := "tfacc-legal-hold-" + acctest.RandString(8)
	objectKey := "reports#2026.csv"
	resourceName := "minio_s3_object_legal_hold.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioObjectLegalHoldDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3ObjectLegalHoldVersionedConfig(bucketName, objectKey, "ON"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectLegalHoldExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "key", objectKey),
					resource.TestCheckResourceAttrSet(resourceName, "version_id"),
					resource.TestCheckResourceAttr(resourceName, "status", "ON"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccImportStateId(resourceName),
			},
		},
	})
}

// Importing an object that never had a legal hold reads the effective status
// instead of failing the plan. See issue #1378.
func TestAccMinioS3ObjectLegalHold_importWithoutHold(t *testing.T) {
	bucketName := "tfacc-legal-hold-" + acctest.RandString(8)
	heldKey := "held.txt"
	plainKey := "plain.txt"
	resourceName := "minio_s3_object_legal_hold.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioObjectLegalHoldDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3ObjectLegalHoldTwoObjectsConfig(bucketName, heldKey, plainKey),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3ObjectLegalHoldExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "status", "ON"),
				),
			},
			{
				ResourceName:  resourceName,
				ImportState:   true,
				ImportStateId: fmt.Sprintf("%s/%s", bucketName, plainKey),
				Check:         resource.TestCheckResourceAttr(resourceName, "status", "OFF"),
			},
		},
	})
}

// testAccImportStateId imports a resource by the ID the provider stored, which is
// what a user re-importing after a state loss has to type.
func testAccImportStateId(n string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return "", fmt.Errorf("not found: %s", n)
		}
		return rs.Primary.ID, nil
	}
}

func testAccMinioS3ObjectLegalHoldVersionedConfig(bucketName, objectKey, status string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket         = "%[1]s"
  object_locking = true
  force_destroy  = true
}

resource "minio_s3_object" "object" {
  bucket_name = minio_s3_bucket.bucket.id
  object_name = "%[2]s"
  content     = "test content"
}

resource "minio_s3_object_legal_hold" "test" {
  bucket     = minio_s3_bucket.bucket.id
  key        = minio_s3_object.object.object_name
  version_id = minio_s3_object.object.version_id
  status     = "%[3]s"
}
`, bucketName, objectKey, status)
}

func testAccMinioS3ObjectLegalHoldTwoObjectsConfig(bucketName, heldKey, plainKey string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket         = "%[1]s"
  object_locking = true
  force_destroy  = true
}

resource "minio_s3_object" "held" {
  bucket_name = minio_s3_bucket.bucket.id
  object_name = "%[2]s"
  content     = "test content"
}

resource "minio_s3_object" "plain" {
  bucket_name = minio_s3_bucket.bucket.id
  object_name = "%[3]s"
  content     = "test content"
}

resource "minio_s3_object_legal_hold" "test" {
  bucket = minio_s3_bucket.bucket.id
  key    = minio_s3_object.held.object_name
  status = "ON"
}
`, bucketName, heldKey, plainKey)
}
