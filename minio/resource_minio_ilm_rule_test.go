package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

func TestAccILMRule_basic(t *testing.T) {
	var lifecycleConfig lifecycle.Configuration
	name := fmt.Sprintf("test-ilm-rule-%d", acctest.RandInt())
	resourceName := "minio_ilm_rule.rule"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioILMRuleConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioS3BucketExists("minio_s3_bucket.bucket"),
					testAccCheckMinioILMRuleExists(resourceName, &lifecycleConfig),
					resource.TestCheckResourceAttr(resourceName, "bucket", name),
					testAccCheckMinioLifecycleConfigurationValid(&lifecycleConfig),
				),
			},
		},
	})
}

func testAccCheckMinioLifecycleConfigurationValid(config *lifecycle.Configuration) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if config.Empty() || len(config.Rules) == 0 {
			return fmt.Errorf("lifecycle configuration is empty")
		}
		return nil
	}
}

func testAccCheckMinioILMRuleExists(n string, config *lifecycle.Configuration) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient).S3Client
		bucketLifecycle, _ := minioC.GetBucketLifecycle(context.Background(), rs.Primary.ID)
		if bucketLifecycle == nil {
			return fmt.Errorf("bucket lifecycle not found")
		}
		*config = *bucketLifecycle

		return nil
	}
}

func testAccMinioILMRuleConfig(randInt string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "bucket" {
  bucket = "%s"
  acl    = "public-read"
}
resource "minio_ilm_rule" "rule" {
  bucket = "${minio_s3_bucket.bucket.id}"
  rules {
	id = "asdf"
	expiration = 7
	filter = "temp/"
  }
}
`, randInt)
}
