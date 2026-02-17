package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioS3BucketQuota_basic(t *testing.T) {
	bucketName := "tfacc-bucket-quota-" + acctest.RandString(8)
	resourceName := "minio_s3_bucket_quota.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioBucketQuotaDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioS3BucketQuotaConfig(bucketName, 1048576),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketQuotaExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "quota", "1048576"),
					resource.TestCheckResourceAttr(resourceName, "type", "hard"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId:     bucketName,
			},
			{
				Config: testAccMinioS3BucketQuotaConfig(bucketName, 2097152),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketQuotaExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "quota", "2097152"),
					resource.TestCheckResourceAttr(resourceName, "type", "hard"),
				),
			},
		},
	})
}

func testAccCheckMinioS3BucketQuotaExists(n string) resource.TestCheckFunc {
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

func testAccCheckMinioBucketQuotaDestroy(s *terraform.State) error {
	client := testAccProvider.Meta().(*S3MinioClient)
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_s3_bucket_quota" {
			continue
		}
		bucket := rs.Primary.ID
		quota, err := client.S3Admin.GetBucketQuota(ctx, bucket)
		if err != nil {
			continue
		}
		if quota.Quota != 0 {
			return fmt.Errorf("bucket quota still exists for %s: %d", bucket, quota.Quota)
		}
	}
	return nil
}

func testAccMinioS3BucketQuotaConfig(bucketName string, quota int) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
}

resource "minio_s3_bucket_quota" "test" {
  bucket = minio_s3_bucket.bucket.id
  quota  = %d
}
`, bucketName, quota)
}
