package minio

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
)

// testAccOIDCPreCheck skips the test when an OIDC-enabled MinIO instance is not configured.
// Set MINIO_OIDC_ENABLED=1 along with MINIO_OIDC_CONFIG_URL, MINIO_OIDC_CLIENT_ID,
// and MINIO_OIDC_CLIENT_SECRET to run these acceptance tests.
//
// MinIO's identity_openid subsystem is not dynamic: a named configuration written
// through the admin API is persisted but stays invisible to every read, and a
// delete is not applied, until the server restarts. These tests therefore restart
// MinIO after each write (via testAccOIDCRestartAndGet) before verifying the
// configuration server-side, which mirrors how the resource is used in practice
// (apply, then restart) and guards the class of regression reported in issue #1014.
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

// testAccOIDCRestartMinio restarts the MinIO server and waits until it is back,
// so that an OIDC configuration written in the current step becomes visible (or a
// deleted one is actually removed). Named OIDC configs only surface after a restart.
func testAccOIDCRestartMinio(ctx context.Context, admin *madmin.AdminClient) error {
	if err := admin.ServiceRestart(ctx); err != nil {
		return fmt.Errorf("triggering MinIO restart: %w", err)
	}

	// Give the server a moment to begin restarting before polling for readiness.
	time.Sleep(3 * time.Second)

	deadline := time.Now().Add(120 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, err := admin.ServerInfo(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("MinIO did not come back after restart: %w", lastErr)
}

// testAccOIDCRestartAndExists restarts MinIO and then asserts the named OIDC
// configuration backing resourceName is present on the server.
func testAccOIDCRestartAndExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("no OIDC IDP configuration ID is set")
		}

		ctx := context.Background()
		admin := testAccProvider.Meta().(*S3MinioClient).S3Admin
		if err := testAccOIDCRestartMinio(ctx, admin); err != nil {
			return err
		}

		if _, err := admin.GetIDPConfig(ctx, madmin.OpenidIDPCfg, rs.Primary.ID); err != nil {
			return fmt.Errorf("OIDC IDP configuration %s not found after restart: %w", rs.Primary.ID, err)
		}
		return nil
	}
}

func testAccCheckMinioIAMIdpOpenIdDestroy(s *terraform.State) error {
	ctx := context.Background()
	admin := testAccProvider.Meta().(*S3MinioClient).S3Admin

	var toCheck []string
	for _, rs := range s.RootModule().Resources {
		if rs.Type == "minio_iam_idp_openid" && rs.Primary.ID != "" {
			toCheck = append(toCheck, rs.Primary.ID)
		}
	}
	if len(toCheck) == 0 {
		return nil
	}

	// A delete only takes effect after a restart, so restart before verifying.
	if err := testAccOIDCRestartMinio(ctx, admin); err != nil {
		return err
	}

	for _, id := range toCheck {
		_, err := admin.GetIDPConfig(ctx, madmin.OpenidIDPCfg, id)
		if err == nil {
			return fmt.Errorf("OIDC IDP configuration %s still exists", id)
		}
		if !isIDPConfigNotFound(err) {
			return fmt.Errorf("unexpected error checking OIDC IDP configuration %s: %w", id, err)
		}
	}
	return nil
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
					resource.TestCheckResourceAttr(resourceName, "name", cfgName),
					resource.TestCheckResourceAttr(resourceName, "config_url", configURL),
					resource.TestCheckResourceAttr(resourceName, "client_id", clientID),
					resource.TestCheckResourceAttr(resourceName, "enable", "true"),
					testAccOIDCRestartAndExists(resourceName),
				),
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
					resource.TestCheckResourceAttr(resourceName, "enable", "true"),
					testAccOIDCRestartAndExists(resourceName),
				),
			},
			{
				Config: testAccMinioIAMIdpOpenIdWithComment(cfgName, configURL, clientID, clientSecret, "updated comment"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "comment", "updated comment"),
					resource.TestCheckResourceAttr(resourceName, "enable", "true"),
					testAccOIDCRestartAndExists(resourceName),
				),
			},
		},
	})
}

func TestAccMinioIAMIdpOpenId_writeOnlyClientSecret(t *testing.T) {
	resourceName := "minio_iam_idp_openid.test"
	cfgName := "tfacc-oidc-wo-" + acctest.RandString(6)
	configURL := os.Getenv("MINIO_OIDC_CONFIG_URL")
	clientID := os.Getenv("MINIO_OIDC_CLIENT_ID")
	clientSecret := os.Getenv("MINIO_OIDC_CLIENT_SECRET")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccOIDCPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMIdpOpenIdDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMIdpOpenIdWriteOnly(cfgName, configURL, clientID, clientSecret, 1),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", cfgName),
					resource.TestCheckResourceAttr(resourceName, "client_id", clientID),
					resource.TestCheckResourceAttr(resourceName, "client_secret_wo_version", "1"),
					testAccOIDCRestartAndExists(resourceName),
				),
			},
		},
	})
}

func TestAccMinioIAMIdpOpenId_writeOnlyClientSecret_transition(t *testing.T) {
	resourceName := "minio_iam_idp_openid.test"
	cfgName := "tfacc-oidc-wo-transition-" + acctest.RandString(6)
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
					resource.TestCheckResourceAttr(resourceName, "client_secret", clientSecret),
					testAccOIDCRestartAndExists(resourceName),
				),
			},
			{
				Config: testAccMinioIAMIdpOpenIdWriteOnly(cfgName, configURL, clientID, clientSecret, 2),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "client_secret_wo_version", "2"),
					testAccOIDCRestartAndExists(resourceName),
				),
			},
			{
				Config: testAccMinioIAMIdpOpenIdBasic(cfgName, configURL, clientID, clientSecret),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "client_secret", clientSecret),
					testAccOIDCRestartAndExists(resourceName),
				),
			},
		},
	})
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

func testAccMinioIAMIdpOpenIdWriteOnly(name, configURL, clientID, clientSecret string, version int) string {
	return fmt.Sprintf(`
resource "minio_iam_idp_openid" "test" {
  name                     = %[1]q
  config_url               = %[2]q
  client_id                = %[3]q
  client_secret_wo         = %[4]q
  client_secret_wo_version = %[5]d
}
`, name, configURL, clientID, clientSecret, version)
}
