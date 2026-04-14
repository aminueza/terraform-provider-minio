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

func testAccILMTierPreCheck(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)

	for _, env := range []string{"SECOND_MINIO_ENDPOINT", "SECOND_MINIO_USER", "SECOND_MINIO_PASSWORD"} {
		if os.Getenv(env) == "" {
			t.Skipf("Skipping ILM tier tests: %s is not set", env)
		}
	}
}

func TestAccMinioILMTier_minioType(t *testing.T) {
	resourceName := "minio_ilm_tier.test"
	tierName := "TFACC" + acctest.RandStringFromCharSet(6, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	bucketName := "tfacc-tier-" + acctest.RandString(6)
	endpoint := os.Getenv("SECOND_MINIO_ENDPOINT")
	accessKey := os.Getenv("SECOND_MINIO_USER")
	secretKey := os.Getenv("SECOND_MINIO_PASSWORD")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccILMTierPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:      testAccCheckMinioILMTierDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioILMTierMinioConfig(tierName, bucketName, endpoint, accessKey, secretKey),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioILMTierExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", tierName),
					resource.TestCheckResourceAttr(resourceName, "type", "minio"),
					resource.TestCheckResourceAttr(resourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(resourceName, "prefix", "tier/"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"force_new_credentials", "minio_config.0.secret_key"},
			},
		},
	})
}

func testAccCheckMinioILMTierExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("no ILM tier ID is set")
		}

		minioC := testMustGetMinioClient()
		tier, err := getTier(minioC.S3Admin, context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error reading tier %s: %w", rs.Primary.ID, err)
		}
		if tier == nil {
			return fmt.Errorf("tier %s not found", rs.Primary.ID)
		}
		return nil
	}
}

func testAccCheckMinioILMTierDestroy(s *terraform.State) error {
	minioC := testMustGetMinioClient()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_ilm_tier" {
			continue
		}

		tier, err := getTier(minioC.S3Admin, context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		if tier != nil {
			return fmt.Errorf("tier %s still exists", rs.Primary.ID)
		}
	}
	return nil
}

func testAccMinioILMTierMinioConfig(name, bucket, endpoint, accessKey, secretKey string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "tier_target" {
  provider = secondminio
  bucket   = %[2]q
}

resource "minio_ilm_tier" "test" {
  name     = %[1]q
  type     = "minio"
  bucket   = minio_s3_bucket.tier_target.bucket
  endpoint = "http://%[3]s"
  prefix   = "tier/"

  minio_config {
    access_key = %[4]q
    secret_key = %[5]q
  }
}
`, name, bucket, endpoint, accessKey, secretKey)
}
