package minio

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v3"
)

// testAccOIDCPreCheck skips the test when an OIDC-enabled MinIO instance is not configured.
// Set MINIO_OIDC_ENABLED=1 along with MINIO_OIDC_CONFIG_URL, MINIO_OIDC_CLIENT_ID,
// and MINIO_OIDC_CLIENT_SECRET to run these acceptance tests.
func testAccOIDCPreCheck(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)

	if v := os.Getenv("MINIO_OIDC_ENABLED"); v != "1" {
		t.Skip("Skipping OIDC acceptance tests: set MINIO_OIDC_ENABLED=1 to run")
	}

	for _, env := range []string{"MINIO_OIDC_CONFIG_URL", "MINIO_OIDC_CLIENT_ID", "MINIO_OIDC_CLIENT_SECRET"} {
		if os.Getenv(env) == "" {
			t.Skipf("Skipping OIDC acceptance tests: %s is not set", env)
		}
	}
}

func TestAccMinioIAMIdpOpenId_basic(t *testing.T) {
	resourceName := "minio_iam_idp_openid.test"
	cfgName := "tfacc-oidc-" + acctest.RandString(6)
	configURL := os.Getenv("MINIO_OIDC_CONFIG_URL")
	clientID := os.Getenv("MINIO_OIDC_CLIENT_ID")
	clientSecret := os.Getenv("MINIO_OIDC_CLIENT_SECRET")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccOIDCPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMIdpOpenIdDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMIdpOpenIdBasic(cfgName, configURL, clientID, clientSecret),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMIdpOpenIdExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", cfgName),
					resource.TestCheckResourceAttr(resourceName, "config_url", configURL),
					resource.TestCheckResourceAttr(resourceName, "client_id", clientID),
					resource.TestCheckResourceAttr(resourceName, "enable", "true"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"client_secret", "restart_required"},
			},
		},
	})
}

func TestAccMinioIAMIdpOpenId_update(t *testing.T) {
	resourceName := "minio_iam_idp_openid.test"
	cfgName := "tfacc-oidc-" + acctest.RandString(6)
	configURL := os.Getenv("MINIO_OIDC_CONFIG_URL")
	clientID := os.Getenv("MINIO_OIDC_CLIENT_ID")
	clientSecret := os.Getenv("MINIO_OIDC_CLIENT_SECRET")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccOIDCPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMIdpOpenIdDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMIdpOpenIdBasic(cfgName, configURL, clientID, clientSecret),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMIdpOpenIdExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "claim_name", "policy"),
					resource.TestCheckResourceAttr(resourceName, "enable", "true"),
				),
			},
			{
				Config: testAccMinioIAMIdpOpenIdWithComment(cfgName, configURL, clientID, clientSecret, "updated comment"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMIdpOpenIdExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "comment", "updated comment"),
					resource.TestCheckResourceAttr(resourceName, "enable", "true"),
				),
			},
		},
	})
}

func testAccCheckMinioIAMIdpOpenIdExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no OIDC IDP configuration ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient)
		_, err := minioC.S3Admin.GetIDPConfig(context.Background(), madmin.OpenidIDPCfg, rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("OIDC IDP configuration %s not found: %w", rs.Primary.ID, err)
		}

		return nil
	}
}

func testAccCheckMinioIAMIdpOpenIdDestroy(s *terraform.State) error {
	minioC := testAccProvider.Meta().(*S3MinioClient)

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_idp_openid" {
			continue
		}

		_, err := minioC.S3Admin.GetIDPConfig(context.Background(), madmin.OpenidIDPCfg, rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("OIDC IDP configuration %s still exists", rs.Primary.ID)
		}
		if !isIDPConfigNotFound(err) {
			return fmt.Errorf("unexpected error checking OIDC IDP configuration %s: %w", rs.Primary.ID, err)
		}
	}

	return nil
}

func testAccMinioIAMIdpOpenIdBasic(name, configURL, clientID, clientSecret string) string {
	return fmt.Sprintf(`
resource "minio_iam_idp_openid" "test" {
  name          = %[1]q
  config_url    = %[2]q
  client_id     = %[3]q
  client_secret = %[4]q
}
`, name, configURL, clientID, clientSecret)
}

func testAccMinioIAMIdpOpenIdWithComment(name, configURL, clientID, clientSecret, comment string) string {
	return fmt.Sprintf(`
resource "minio_iam_idp_openid" "test" {
  name          = %[1]q
  config_url    = %[2]q
  client_id     = %[3]q
  client_secret = %[4]q
  comment       = %[5]q
}
`, name, configURL, clientID, clientSecret, comment)
}
