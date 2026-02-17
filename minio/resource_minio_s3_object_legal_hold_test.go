package minio

import (
	"context"
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
	client := testAccProvider.Meta().(*S3MinioClient)
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_s3_object_legal_hold" {
			continue
		}
		bucket, key := parseBucketAndKeyFromID(rs.Primary.ID)
		if bucket == "" || key == "" {
			return fmt.Errorf("invalid legal hold ID format: %s", rs.Primary.ID)
		}
		status, err := client.S3Client.GetObjectLegalHold(ctx, bucket, key, minio.GetObjectLegalHoldOptions{})
		if err != nil {
			continue
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
