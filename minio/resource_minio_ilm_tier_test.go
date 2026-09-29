package minio

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// The config blocks are Optional in the schema, so a tier whose type names a
// backend without its matching block used to panic the provider on create and
// update (issue #1200). These unit tests need no MinIO server: the guard must
// return a diagnostic before any API call, and removing the guard makes them
// fail with an index-out-of-range panic.
func TestILMTierConfigBlockMissing(t *testing.T) {
	res := resourceMinioILMTier()

	for _, tierType := range []string{"s3", "minio", "gcs", "azure"} {
		configKey := tierType + "_config"

		t.Run("create_"+tierType, func(t *testing.T) {
			d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
				"name":   "TFTIER",
				"type":   tierType,
				"bucket": "cold-storage",
			})
			diags := minioCreateILMTier(context.Background(), d, &S3MinioClient{})
			assertMissingBlockError(t, diags, configKey, tierType)
			if d.Id() != "" {
				t.Errorf("expected no resource ID after a failed create, got %q", d.Id())
			}
		})

		t.Run("update_"+tierType, func(t *testing.T) {
			d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
				"name":   "TFTIER",
				"type":   tierType,
				"bucket": "cold-storage",
			})
			d.SetId("TFTIER")
			assertMissingBlockError(t, minioUpdateILMTier(context.Background(), d, &S3MinioClient{}), configKey, tierType)
		})
	}
}

func TestILMTierConfigBlockPresent(t *testing.T) {
	res := resourceMinioILMTier()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"name":   "TFTIER",
		"type":   "s3",
		"bucket": "cold-storage",
		"s3_config": []interface{}{
			map[string]interface{}{
				"access_key":    "AKIAEXAMPLE",
				"secret_key":    "secret",
				"storage_class": "GLACIER",
			},
		},
	})

	block, diags := ilmTierConfigBlock(d, "s3_config")
	if diags.HasError() {
		t.Fatalf("expected no error for a present block, got: %v", diags)
	}
	if block["access_key"] != "AKIAEXAMPLE" {
		t.Errorf("expected access_key %q, got %v", "AKIAEXAMPLE", block["access_key"])
	}
	if block["storage_class"] != "GLACIER" {
		t.Errorf("expected storage_class %q, got %v", "GLACIER", block["storage_class"])
	}
}

func assertMissingBlockError(t *testing.T, diags diag.Diagnostics, configKey, tierType string) {
	t.Helper()
	if !diags.HasError() {
		t.Fatalf("expected an error diagnostic for missing %s, got: %v", configKey, diags)
	}
	want := fmt.Sprintf("%s is required when type is %s", configKey, tierType)
	if !strings.Contains(diags[0].Summary, want) {
		t.Errorf("expected summary to contain %q, got: %q", want, diags[0].Summary)
	}
}

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
		ProviderFactories: testAccProviders,
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

		minioC := testAccClient()
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
	minioC := testAccClient()

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
