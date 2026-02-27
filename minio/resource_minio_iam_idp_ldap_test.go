package minio

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v3"
)

func testAccLDAPPreCheck(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)

	if v := os.Getenv("MINIO_LDAP_ENABLED"); v != "1" {
		t.Skip("Skipping LDAP acceptance tests: set MINIO_LDAP_ENABLED=1 to run")
	}

	if os.Getenv("MINIO_LDAP_SERVER_ADDR") == "" {
		t.Skip("Skipping LDAP acceptance tests: MINIO_LDAP_SERVER_ADDR is not set")
	}
}

func TestAccMinioIAMIdpLdap_basic(t *testing.T) {
	resourceName := "minio_iam_idp_ldap.test"
	serverAddr := os.Getenv("MINIO_LDAP_SERVER_ADDR")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccLDAPPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMIdpLdapDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMIdpLdapBasic(serverAddr),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMIdpLdapExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "server_addr", serverAddr),
					resource.TestCheckResourceAttr(resourceName, "enable", "true"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"lookup_bind_password", "restart_required"},
			},
		},
	})
}

func TestAccMinioIAMIdpLdap_update(t *testing.T) {
	resourceName := "minio_iam_idp_ldap.test"
	serverAddr := os.Getenv("MINIO_LDAP_SERVER_ADDR")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccLDAPPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMIdpLdapDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMIdpLdapBasic(serverAddr),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMIdpLdapExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "tls_skip_verify", "false"),
				),
			},
			{
				Config: testAccMinioIAMIdpLdapWithTLSSkipVerify(serverAddr),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMIdpLdapExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "tls_skip_verify", "true"),
				),
			},
		},
	})
}

func testAccCheckMinioIAMIdpLdapExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no LDAP IDP configuration ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient)
		_, err := minioC.S3Admin.GetIDPConfig(context.Background(), madmin.LDAPIDPCfg, madmin.Default)
		if err != nil {
			return fmt.Errorf("LDAP IDP configuration not found: %w", err)
		}

		return nil
	}
}

func testAccCheckMinioIAMIdpLdapDestroy(s *terraform.State) error {
	minioC := testAccProvider.Meta().(*S3MinioClient)

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_idp_ldap" {
			continue
		}

		_, err := minioC.S3Admin.GetIDPConfig(context.Background(), madmin.LDAPIDPCfg, madmin.Default)
		if err == nil {
			return fmt.Errorf("LDAP IDP configuration still exists")
		}
		if !isIDPConfigNotFound(err) {
			return fmt.Errorf("unexpected error checking LDAP IDP configuration: %w", err)
		}
	}

	return nil
}

func testAccMinioIAMIdpLdapBasic(serverAddr string) string {
	return fmt.Sprintf(`
resource "minio_iam_idp_ldap" "test" {
  server_addr = %[1]q
}
`, serverAddr)
}

func testAccMinioIAMIdpLdapWithTLSSkipVerify(serverAddr string) string {
	return fmt.Sprintf(`
resource "minio_iam_idp_ldap" "test" {
  server_addr     = %[1]q
  tls_skip_verify = true
}
`, serverAddr)
}
