package minio

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
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

func TestAccMinioIAMIdpLdap_regression902(t *testing.T) {
	resourceName := "minio_iam_idp_ldap.test"
	serverAddr := os.Getenv("MINIO_LDAP_SERVER_ADDR")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccLDAPPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMIdpLdapBoolVariant(serverAddr, false, false, true, true),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMIdpLdapExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "starttls", "false"),
					resource.TestCheckResourceAttr(resourceName, "tls_skip_verify", "false"),
					resource.TestCheckResourceAttr(resourceName, "server_insecure", "true"),
				),
			},
		},
	})
}

func TestAccMinioIAMIdpLdap_roundTripReadBack(t *testing.T) {
	resourceName := "minio_iam_idp_ldap.test"
	serverAddr := os.Getenv("MINIO_LDAP_SERVER_ADDR")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccLDAPPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMIdpLdapBasic(serverAddr),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMIdpLdapExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "server_addr", serverAddr),
					resource.TestCheckResourceAttr(resourceName, "lookup_bind_dn", "cn=admin,dc=example,dc=com"),
					resource.TestCheckResourceAttr(resourceName, "user_dn_search_base_dn", "ou=users,dc=example,dc=com"),
					resource.TestCheckResourceAttr(resourceName, "user_dn_search_filter", "(cn=%s)"),
					resource.TestCheckResourceAttr(resourceName, "group_search_base_dn", "ou=groups,dc=example,dc=com"),
					resource.TestCheckResourceAttr(resourceName, "server_insecure", "true"),
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
		raw, err := minioC.S3Admin.GetConfigKV(context.Background(), "identity_ldap")
		if err != nil {
			return fmt.Errorf("reading identity_ldap config: %w", err)
		}
		kv := strings.TrimSpace(string(raw))
		if !strings.Contains(kv, "server_addr=") || strings.Contains(kv, "server_addr= ") {
			return fmt.Errorf("identity_ldap.server_addr is empty; config did not persist: %s", kv)
		}
		return nil
	}
}

func testAccMinioIAMIdpLdapBasic(serverAddr string) string {
	return fmt.Sprintf(`
resource "minio_iam_idp_ldap" "test" {
  server_addr            = %[1]q
  lookup_bind_dn         = "cn=admin,dc=example,dc=com"
  lookup_bind_password   = "adminpassword"
  user_dn_search_base_dn = "ou=users,dc=example,dc=com"
  user_dn_search_filter  = "(cn=%%s)"
  group_search_base_dn   = "ou=groups,dc=example,dc=com"
  group_search_filter    = "(&(objectclass=groupOfNames)(member=%%d))"
  server_insecure        = true
}
`, serverAddr)
}

func testAccMinioIAMIdpLdapBoolVariant(serverAddr string, starttls, tlsSkipVerify, serverInsecure, enable bool) string {
	return fmt.Sprintf(`
resource "minio_iam_idp_ldap" "test" {
  server_addr            = %[1]q
  lookup_bind_dn         = "cn=admin,dc=example,dc=com"
  lookup_bind_password   = "adminpassword"
  user_dn_search_base_dn = "ou=users,dc=example,dc=com"
  user_dn_search_filter  = "(cn=%%s)"
  group_search_base_dn   = "ou=groups,dc=example,dc=com"
  group_search_filter    = "(&(objectclass=groupOfNames)(member=%%d))"
  starttls               = %[2]t
  tls_skip_verify        = %[3]t
  server_insecure        = %[4]t
  enable                 = %[5]t
}
`, serverAddr, starttls, tlsSkipVerify, serverInsecure, enable)
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
				"server_starttls=off",
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

func TestBuildIdpLdapCfgData_minimalServerAddrOnly(t *testing.T) {
	got := buildIdpLdapCfgData(&S3MinioIdpLdap{
		ServerAddr: "openldap:389",
		Enable:     true,
	})
	want := "server_addr=openldap:389 tls_skip_verify=off server_insecure=off server_starttls=off enable=on"
	if got != want {
		t.Fatalf("minimal config mismatch\nwant: %s\n got: %s", want, got)
	}
}

func TestBuildIdpLdapCfgData_regression902(t *testing.T) {
	got := buildIdpLdapCfgData(&S3MinioIdpLdap{
		ServerAddr:        "openldap:389",
		GroupSearchFilter: "(&(objectclass=groupOfNames)(member=%d))",
		Enable:            true,
	})
	if !strings.Contains(got, "server_starttls=") {
		t.Errorf("expected server_starttls wire key, got: %s", got)
	}
	if strings.Contains(got, " starttls=") || strings.HasPrefix(got, "starttls=") {
		t.Errorf("expected no bare 'starttls' key (must be server_starttls), got: %s", got)
	}
}
