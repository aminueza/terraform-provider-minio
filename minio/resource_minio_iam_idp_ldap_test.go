package minio

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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

// testAccCleanupIdpLdap removes the identity_ldap config and restarts the
// primary server so a test that ends before its own destroy leaves the
// shared server with LDAP off: an active server refuses to create the
// built-in IAM users later tests need. The SDK skips CheckDestroy in that
// case, so this is the safety net; testAccCheckMinioIAMIdpLdapRemoved stays
// the check. It is registered from PreCheck, after the LDAP checks pass, so
// it runs only when the test will run: with TF_ACC unset or the LDAP
// variables missing the test is skipped before the cleanup is registered.
func testAccCleanupIdpLdap(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		admin := testAccClient().S3Admin
		if _, err := admin.DelConfigKV(context.Background(), "identity_ldap"); err != nil {
			// The test's own destroy already removed the config, in which
			// case the delete reports it as missing.
			if !isIDPConfigNotFound(err) && !strings.Contains(strings.ToLower(err.Error()), "there is no target") {
				t.Errorf("removing identity_ldap after the test: %v", err)
			}
		}
		if err := testAccRestartPrimaryServer(); err != nil {
			t.Errorf("restarting the primary server after the test: %v", err)
		}
	})
}

func TestAccMinioIAMIdpLdap_basic(t *testing.T) {
	resourceName := "minio_iam_idp_ldap.test"
	serverAddr := os.Getenv("MINIO_LDAP_SERVER_ADDR")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccLDAPPreCheck(t); testAccCleanupIdpLdap(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMIdpLdapRemoved(),
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
		PreCheck:          func() { testAccLDAPPreCheck(t); testAccCleanupIdpLdap(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMIdpLdapRemoved(),
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
		PreCheck:          func() { testAccLDAPPreCheck(t); testAccCleanupIdpLdap(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMIdpLdapRemoved(),
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

func TestAccMinioIAMIdpLdap_enableFalse(t *testing.T) {
	resourceName := "minio_iam_idp_ldap.test"
	serverAddr := os.Getenv("MINIO_LDAP_SERVER_ADDR")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccLDAPPreCheck(t); testAccCleanupIdpLdap(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMIdpLdapRemoved(),
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMIdpLdapBoolVariant(serverAddr, false, false, true, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
					testAccCheckMinioIAMIdpLdapEnableOff(),
				),
			},
		},
	})
}

// TestAccMinioIAMIdpLdap_enableFalseToTrue turns LDAP on after it was off.
// Until the restart, the server still reports enable=off, so the read right
// after the apply must keep enable = true from the plan instead of loading
// the stale value back, and the refresh after the restart must clear
// restart_required.
func TestAccMinioIAMIdpLdap_enableFalseToTrue(t *testing.T) {
	resourceName := "minio_iam_idp_ldap.test"
	serverAddr := os.Getenv("MINIO_LDAP_SERVER_ADDR")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccLDAPPreCheck(t); testAccCleanupIdpLdap(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMIdpLdapRemoved(),
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMIdpLdapBoolVariant(serverAddr, false, false, true, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
					testAccCheckMinioIAMIdpLdapEnableOff(),
				),
			},
			{
				Config: testAccMinioIAMIdpLdapBoolVariant(serverAddr, false, false, true, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
					resource.TestCheckResourceAttr(resourceName, "restart_required", "false"),
				),
			},
			{
				Config: testAccMinioIAMIdpLdapBoolVariant(serverAddr, false, false, true, true),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enable", "true"),
					resource.TestCheckResourceAttr(resourceName, "restart_required", "true"),
					testAccCheckMinioIAMIdpLdapExists(resourceName),
				),
			},
			{
				Config: testAccMinioIAMIdpLdapBoolVariant(serverAddr, false, false, true, true),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enable", "true"),
					resource.TestCheckResourceAttr(resourceName, "restart_required", "false"),
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

		// get-config-kv only serves a changed identity_ldap after a restart,
		// so the check restarts the primary server first. The check must only
		// run in sequential tests: see testAccRestartPrimaryServer.
		if err := testAccRestartPrimaryServer(); err != nil {
			return err
		}

		minioC := testAccClient()
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

// testAccCheckMinioIAMIdpLdapRemoved restarts the primary server and fails
// unless get-config-kv reports identity_ldap with an empty server_addr. A
// reset, like a change, is only visible after the restart, and a server left
// with LDAP active refuses to create built-in IAM users, which would break
// every later test that makes one.
func testAccCheckMinioIAMIdpLdapRemoved() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if err := testAccRestartPrimaryServer(); err != nil {
			return err
		}

		minioC := testAccClient()
		raw, err := minioC.S3Admin.GetConfigKV(context.Background(), "identity_ldap")
		if err != nil {
			return fmt.Errorf("reading identity_ldap config after destroy: %w", err)
		}
		cfgStr := strings.TrimPrefix(strings.TrimSpace(string(raw)), "identity_ldap ")
		cfgMap := parseConfigParams(cfgStr)
		if addr := cfgMap["server_addr"]; addr != "" {
			return fmt.Errorf("identity_ldap.server_addr is %q after destroy; LDAP is still active: %s", addr, string(raw))
		}
		return nil
	}
}

// testAccCheckMinioIAMIdpLdapEnableOff restarts the primary server and fails
// unless get-config-kv reports identity_ldap with enable=off. The server
// prints the enable key only when it is off.
func testAccCheckMinioIAMIdpLdapEnableOff() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if err := testAccRestartPrimaryServer(); err != nil {
			return err
		}

		minioC := testAccClient()
		raw, err := minioC.S3Admin.GetConfigKV(context.Background(), "identity_ldap")
		if err != nil {
			return fmt.Errorf("reading identity_ldap config after the restart: %w", err)
		}
		cfgStr := strings.TrimPrefix(strings.TrimSpace(string(raw)), "identity_ldap ")
		cfgMap := parseConfigParams(cfgStr)
		if got := cfgMap["enable"]; got != "off" {
			return fmt.Errorf("identity_ldap.enable is %q after the restart, want %q: %s", got, "off", string(raw))
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

// TestIdpLdapConfigEnable covers issue #1305: GetOk treats enable = false as
// unset and substituted the default true, so the flag was always sent enabled.
func TestIdpLdapConfigEnable(t *testing.T) {
	res := resourceMinioIAMIdpLdap()

	t.Run("enable false is not treated as unset", func(t *testing.T) {
		d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
			"server_addr": "ldap.example.com:389",
			"enable":      false,
		})
		if IdpLdapConfig(d, &S3MinioClient{}).Enable {
			t.Errorf("expected Enable false when enable = false, got true")
		}
	})

	t.Run("enable unset defaults to true", func(t *testing.T) {
		d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
			"server_addr": "ldap.example.com:389",
		})
		if !IdpLdapConfig(d, &S3MinioClient{}).Enable {
			t.Errorf("expected Enable true when enable is unset, got false")
		}
	})
}

func TestIdpLdapRunningConfigMatchesState(t *testing.T) {
	config := &S3MinioIdpLdap{
		ServerAddr:         "openldap:389",
		LookupBindDN:       "cn=admin,dc=example,dc=com",
		LookupBindPassword: "adminpassword",
		UserDNSearchBaseDN: "ou=users,dc=example,dc=com",
		UserDNSearchFilter: "(cn=%s)",
		GroupSearchBaseDN:  "ou=groups,dc=example,dc=com",
		GroupSearchFilter:  "(&(objectclass=groupOfNames)(member=%d))",
		ServerInsecure:     true,
		Enable:             true,
	}

	// The maps below are the responses measured from MinIO, not the output
	// of buildIdpLdapCfgData: srv_record_name is printed by the server even
	// though the provider never sends it, and lookup_bind_password is never
	// printed at all.
	t.Run("pre-restart response with every key empty does not match", func(t *testing.T) {
		if idpLdapRunningConfigMatchesState(config, map[string]string{
			"server_addr":            "",
			"lookup_bind_dn":         "",
			"user_dn_search_base_dn": "",
			"user_dn_search_filter":  "",
			"group_search_base_dn":   "",
			"group_search_filter":    "",
			"tls_skip_verify":        "",
			"server_insecure":        "",
			"server_starttls":        "",
			"enable":                 "",
			"srv_record_name":        "",
		}) {
			t.Errorf("expected no match for the pre-restart response")
		}
	})

	t.Run("post-restart response without enable key matches", func(t *testing.T) {
		if !idpLdapRunningConfigMatchesState(config, map[string]string{
			"server_addr":            "openldap:389",
			"lookup_bind_dn":         "cn=admin,dc=example,dc=com",
			"user_dn_search_base_dn": "ou=users,dc=example,dc=com",
			"user_dn_search_filter":  "(cn=%s)",
			"group_search_base_dn":   "ou=groups,dc=example,dc=com",
			"group_search_filter":    "(&(objectclass=groupOfNames)(member=%d))",
			"tls_skip_verify":        "off",
			"server_insecure":        "on",
			"server_starttls":        "off",
			"srv_record_name":        "",
		}) {
			t.Errorf("expected the post-restart response to match")
		}
	})

	t.Run("stale value does not match", func(t *testing.T) {
		if idpLdapRunningConfigMatchesState(config, map[string]string{
			"server_addr":            "openldap:389",
			"lookup_bind_dn":         "cn=admin,dc=example,dc=com",
			"user_dn_search_base_dn": "ou=users,dc=example,dc=com",
			"user_dn_search_filter":  "(cn=%s)",
			"group_search_base_dn":   "ou=groups,dc=example,dc=com",
			"group_search_filter":    "(&(objectclass=groupOfNames)(member=%d))",
			"tls_skip_verify":        "off",
			"server_insecure":        "off",
			"server_starttls":        "off",
		}) {
			t.Errorf("expected no match for a stale server_insecure value")
		}
	})

	disabled := *config
	disabled.Enable = false

	t.Run("disabled config with enable=off matches", func(t *testing.T) {
		if !idpLdapRunningConfigMatchesState(&disabled, map[string]string{
			"server_addr":            "openldap:389",
			"lookup_bind_dn":         "cn=admin,dc=example,dc=com",
			"user_dn_search_base_dn": "ou=users,dc=example,dc=com",
			"user_dn_search_filter":  "(cn=%s)",
			"group_search_base_dn":   "ou=groups,dc=example,dc=com",
			"group_search_filter":    "(&(objectclass=groupOfNames)(member=%d))",
			"tls_skip_verify":        "off",
			"server_insecure":        "on",
			"server_starttls":        "off",
			"enable":                 "off",
		}) {
			t.Errorf("expected the disabled post-restart response to match")
		}
	})

	t.Run("disabled config without the enable key does not match", func(t *testing.T) {
		if idpLdapRunningConfigMatchesState(&disabled, map[string]string{
			"server_addr":            "openldap:389",
			"lookup_bind_dn":         "cn=admin,dc=example,dc=com",
			"user_dn_search_base_dn": "ou=users,dc=example,dc=com",
			"user_dn_search_filter":  "(cn=%s)",
			"group_search_base_dn":   "ou=groups,dc=example,dc=com",
			"group_search_filter":    "(&(objectclass=groupOfNames)(member=%d))",
			"tls_skip_verify":        "off",
			"server_insecure":        "on",
			"server_starttls":        "off",
		}) {
			t.Errorf("expected no match: enable=off must come back as off")
		}
	})

	t.Run("enabled config while the server still says enable=off does not match", func(t *testing.T) {
		if idpLdapRunningConfigMatchesState(config, map[string]string{
			"server_addr":            "openldap:389",
			"lookup_bind_dn":         "cn=admin,dc=example,dc=com",
			"user_dn_search_base_dn": "ou=users,dc=example,dc=com",
			"user_dn_search_filter":  "(cn=%s)",
			"group_search_base_dn":   "ou=groups,dc=example,dc=com",
			"group_search_filter":    "(&(objectclass=groupOfNames)(member=%d))",
			"tls_skip_verify":        "off",
			"server_insecure":        "on",
			"server_starttls":        "off",
			"enable":                 "off",
		}) {
			t.Errorf("expected no match: going from enable = false to true must wait for the server to drop enable=off")
		}
	})
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
