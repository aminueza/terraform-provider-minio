package minio

import (
	"context"
	"fmt"
	"os"
	"strings"
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
  server_addr            = %[1]q
  lookup_bind_dn         = "cn=readonly,dc=example,dc=com"
  user_dn_search_base_dn = "ou=users,dc=example,dc=com"
  user_dn_search_filter  = "(uid=%%s)"
  group_search_base_dn   = "ou=groups,dc=example,dc=com"
  group_search_filter    = "(&(objectclass=groupOfNames)(member=%%d))"
}
`, serverAddr)
}

func testAccMinioIAMIdpLdapWithTLSSkipVerify(serverAddr string) string {
	return fmt.Sprintf(`
resource "minio_iam_idp_ldap" "test" {
  server_addr            = %[1]q
  lookup_bind_dn         = "cn=readonly,dc=example,dc=com"
  user_dn_search_base_dn = "ou=users,dc=example,dc=com"
  user_dn_search_filter  = "(uid=%%s)"
  group_search_base_dn   = "ou=groups,dc=example,dc=com"
  group_search_filter    = "(&(objectclass=groupOfNames)(member=%%d))"
  tls_skip_verify        = true
}
`, serverAddr)
}

func TestBuildIdpLdapCfgData(t *testing.T) {
	tests := []struct {
		name     string
		config   S3MinioIdpLdap
		contains []string
		absent   []string
	}{
		{
			name: "basic fields",
			config: S3MinioIdpLdap{
				ServerAddr:    "ldap.example.com:636",
				LookupBindDN:  "cn=admin,dc=example,dc=com",
				TLSSkipVerify: true,
				Enable:        true,
			},
			contains: []string{
				"server_addr=ldap.example.com:636",
				"lookup_bind_dn=cn=admin,dc=example,dc=com",
				"tls_skip_verify=on",
				"enable=on",
				"server_insecure=off",
				"starttls=off",
			},
		},
		{
			name: "value with spaces is quoted",
			config: S3MinioIdpLdap{
				ServerAddr:   "ldap.example.com:389",
				LookupBindDN: "cn=Service Account,dc=example,dc=com",
				Enable:       true,
			},
			contains: []string{`lookup_bind_dn="cn=Service Account,dc=example,dc=com"`},
		},
		{
			name: "empty optional fields are omitted",
			config: S3MinioIdpLdap{
				ServerAddr: "ldap.example.com:389",
				Enable:     true,
			},
			absent: []string{"lookup_bind_dn=", "lookup_bind_password="},
		},
		{
			name: "disabled config",
			config: S3MinioIdpLdap{
				ServerAddr: "ldap.example.com:389",
				Enable:     false,
			},
			contains: []string{"enable=off"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildIdpLdapCfgData(&tc.config)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in config data, got: %s", want, got)
				}
			}
			for _, unwanted := range tc.absent {
				if strings.Contains(got, unwanted) {
					t.Errorf("expected %q to be absent in config data, got: %s", unwanted, got)
				}
			}
		})
	}
}
