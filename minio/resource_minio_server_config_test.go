package minio

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccMinioServerConfigRegion_basic(t *testing.T) {
	resourceName := "minio_server_config_region.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_server_config_region" "test" {
  name = "us-east-1"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "us-east-1"),
				),
			},
			{
				Config: `
resource "minio_server_config_region" "test" {
  name = "eu-west-1"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "eu-west-1"),
				),
			},
		},
	})
}

func TestAccMinioServerConfigApi_basic(t *testing.T) {
	resourceName := "minio_server_config_api.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_server_config_api" "test" {
  stale_uploads_expiry = "12h"
  sync_events          = true
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "stale_uploads_expiry", "12h"),
					resource.TestCheckResourceAttr(resourceName, "sync_events", "true"),
				),
			},
			{
				Config: `
resource "minio_server_config_api" "test" {
  stale_uploads_expiry = "24h"
  sync_events          = false
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "stale_uploads_expiry", "24h"),
					resource.TestCheckResourceAttr(resourceName, "sync_events", "false"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"restart_required"},
			},
		},
	})
}

func TestAccMinioServerConfigScanner_basic(t *testing.T) {
	resourceName := "minio_server_config_scanner.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_server_config_scanner" "test" {
  speed = "slow"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "speed", "slow"),
				),
			},
			{
				Config: `
resource "minio_server_config_scanner" "test" {
  speed = "default"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "speed", "default"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"restart_required"},
			},
		},
	})
}

func TestAccMinioServerConfigHeal_basic(t *testing.T) {
	resourceName := "minio_server_config_heal.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_server_config_heal" "test" {
  bitrotscan = "off"
  max_sleep  = "500ms"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "bitrotscan", "off"),
					resource.TestCheckResourceAttr(resourceName, "max_sleep", "500ms"),
				),
			},
			{
				Config: `
resource "minio_server_config_heal" "test" {
  bitrotscan = "on"
  max_sleep  = "250ms"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "bitrotscan", "on"),
					resource.TestCheckResourceAttr(resourceName, "max_sleep", "250ms"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"restart_required"},
			},
		},
	})
}

func TestAccMinioServerConfigStorageClass_basic(t *testing.T) {
	resourceName := "minio_server_config_storage_class.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `resource "minio_server_config_storage_class" "test" {}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"restart_required"},
			},
		},
	})
}

// TestAccMinioServerConfigEtcd_basic requires TF_ACC_ETCD_ENDPOINTS to be set.
// No etcd is available in the standard CI environment, so this is skipped by default.
func TestAccMinioServerConfigEtcd_basic(t *testing.T) {
	endpoints := os.Getenv("TF_ACC_ETCD_ENDPOINTS")
	if endpoints == "" {
		t.Skip("TF_ACC_ETCD_ENDPOINTS not set — skipping etcd config acceptance test")
	}

	resourceName := "minio_server_config_etcd.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_server_config_etcd" "test" {
  endpoints = "` + endpoints + `"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "endpoints", endpoints),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"restart_required"},
			},
		},
	})
}

// The policy_plugin subsystem is a singleton: every test writes the same
// server config, so these tests stay sequential resource.Test calls and must
// not run in parallel with each other. While the plugin is configured and
// the server has restarted, MinIO sends every authorization decision to the
// plugin URL, so a down or denying endpoint blocks the whole suite: the stub
// below answers allow, and testAccCleanupPolicyPlugin removes the config and
// restarts the server however a test ends.
func TestAccMinioServerConfigPolicyPlugin_basic(t *testing.T) {
	resourceName := "minio_server_config_policy_plugin.test"
	stubURL := testAccPolicyPluginStub(t)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccCleanupPolicyPlugin(t)
		},
		ProviderFactories: testAccProviders,
		// The subsystem cannot be deleted, only reset: after the destroy the
		// server is back to the policy plugin defaults seeded by MinIO.
		CheckDestroy: testAccCheckServerStoredTarget("policy_plugin", "", map[string]string{
			"url":          "",
			"auth_token":   "",
			"enable_http2": "off",
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccMinioServerConfigPolicyPluginBasic(stubURL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "url", stubURL),
					resource.TestCheckResourceAttr(resourceName, "auth_token", "stub-token"),
					resource.TestCheckResourceAttr(resourceName, "enable_http2", "true"),
					resource.TestCheckResourceAttr(resourceName, "comment", "terraform acceptance test"),
					// The server redacts a non-empty auth_token, so the
					// stored config carries no auth_token key here.
					testAccCheckServerStoredTarget("policy_plugin", "", map[string]string{
						"url":          stubURL,
						"enable_http2": "on",
						"comment":      "terraform acceptance test",
					}),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"auth_token", "restart_required"},
			},
		},
	})
}

func TestAccMinioServerConfigPolicyPlugin_update(t *testing.T) {
	resourceName := "minio_server_config_policy_plugin.test"
	stubURL := testAccPolicyPluginStub(t)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccCleanupPolicyPlugin(t)
		},
		ProviderFactories: testAccProviders,
		CheckDestroy: testAccCheckServerStoredTarget("policy_plugin", "", map[string]string{
			"url":          "",
			"auth_token":   "",
			"enable_http2": "off",
		}),
		Steps: []resource.TestStep{
			{
				Config: testAccMinioServerConfigPolicyPluginBasic(stubURL),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckServerStoredTarget("policy_plugin", "", map[string]string{
						"url":          stubURL,
						"enable_http2": "on",
						"comment":      "terraform acceptance test",
					}),
				),
			},
			{
				// Drop auth_token and comment: the resource owns the keys, so
				// the create's values must be cleared, not kept.
				Config: fmt.Sprintf(`
resource "minio_server_config_policy_plugin" "test" {
  url = %[1]q
}
`, stubURL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "auth_token", ""),
					resource.TestCheckResourceAttr(resourceName, "enable_http2", "false"),
					resource.TestCheckResourceAttr(resourceName, "comment", ""),
					testAccCheckServerStoredTarget("policy_plugin", "", map[string]string{
						"url":          stubURL,
						"auth_token":   "",
						"enable_http2": "off",
						"comment":      "",
					}),
				),
			},
		},
	})
}

// A comment set outside Terraform must not survive a create that leaves the
// field empty: the resource owns the singleton, so the create clears it.
func TestAccMinioServerConfigPolicyPlugin_existingCommentCleared(t *testing.T) {
	resourceName := "minio_server_config_policy_plugin.test"
	stubURL := testAccPolicyPluginStub(t)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccCleanupPolicyPlugin(t)
		},
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				PreConfig: func() { testAccSeedPolicyPluginComment(t, stubURL) },
				Config: fmt.Sprintf(`
resource "minio_server_config_policy_plugin" "test" {
  url = %[1]q
}
`, stubURL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "comment", ""),
					testAccCheckServerStoredTarget("policy_plugin", "", map[string]string{
						"comment": "",
					}),
				),
			},
			{
				// The create cleared the seeded comment, so the next plan is
				// empty.
				Config: fmt.Sprintf(`
resource "minio_server_config_policy_plugin" "test" {
  url = %[1]q
}
`, stubURL),
				PlanOnly: true,
			},
		},
	})
}

// testAccPolicyPluginStub runs an allow-everything policy plugin endpoint and
// returns a URL the MinIO test container can reach. MinIO only talks to the
// endpoint once the server has restarted with the plugin configured, and
// then on every authorization decision, so the stub must stay up until after
// the cleanup restart. It closes last because it is registered before the
// cleanup, and t.Cleanup runs in reverse order.
func testAccPolicyPluginStub(t *testing.T) string {
	t.Helper()

	// The acceptance tests that use the stub are skipped by resource.Test
	// unless TF_ACC is set, so there is nothing to serve without it.
	if os.Getenv("TF_ACC") == "" {
		return ""
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result": true}`))
	}))
	ln, err := net.Listen("tcp", testAccPolicyPluginStubAddress(t))
	if err != nil {
		srv.Close()
		t.Fatalf("listening for the policy plugin stub: %v", err)
	}
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)

	return srv.URL
}

// testAccPolicyPluginStubAddress is the address of the test process on the
// compose network: the tests run inside the compose test container, whose
// own IP docker writes into /etc/hosts next to the container name, and the
// MinIO container is on the same network.
func testAccPolicyPluginStubAddress(t *testing.T) string {
	t.Helper()

	hostname, err := os.Hostname()
	if err != nil {
		t.Fatalf("reading the hostname: %v", err)
	}
	data, err := os.ReadFile("/etc/hosts")
	if err != nil {
		t.Fatalf("reading /etc/hosts: %v", err)
	}
	addr := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == hostname && net.ParseIP(fields[0]) != nil {
			addr = fields[0] + ":0"
			break
		}
	}
	if addr == "" {
		t.Fatalf("no IP for %q in /etc/hosts: the policy plugin tests need the compose test container", hostname)
	}
	return addr
}

// testAccCleanupPolicyPlugin removes the policy plugin configuration and
// restarts the primary server however the test ends: while the plugin is
// configured and active, MinIO sends every authorization decision to the
// plugin URL, so a configuration left behind blocks every later test. It is
// registered from PreCheck, after the precheck passes, so it runs only when
// the test runs.
func testAccCleanupPolicyPlugin(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		admin := testAccClient().S3Admin
		if _, err := admin.DelConfigKV(context.Background(), serverConfigPolicyPluginKey); err != nil {
			// The test's own destroy already removed the config, in which
			// case the delete reports it as missing.
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "not found") && !strings.Contains(msg, "there is no target") {
				t.Errorf("removing %s after the test: %v", serverConfigPolicyPluginKey, err)
			}
		}
		if err := testAccRestartPrimaryServer(); err != nil {
			t.Errorf("restarting the primary server after the test: %v", err)
		}
	})
}

func testAccSeedPolicyPluginComment(t *testing.T, stubURL string) {
	t.Helper()
	// The subsystem requires url in the same set-config-kv, so the seed
	// points the plugin at the allow-everything stub: the restart below
	// then runs with the plugin active, and every decision goes to the stub.
	configString := fmt.Sprintf(`policy_plugin url=%s comment="seeded outside terraform"`, stubURL)
	if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), configString); err != nil {
		t.Fatalf("seeding the policy plugin comment outside Terraform: %v", err)
	}
	// The seed only matters after a restart, like any other config change.
	if err := testAccRestartPrimaryServer(); err != nil {
		t.Fatalf("restarting the server: %v", err)
	}
}

func testAccMinioServerConfigPolicyPluginBasic(stubURL string) string {
	return fmt.Sprintf(`
resource "minio_server_config_policy_plugin" "test" {
  url          = %[1]q
  auth_token   = "stub-token"
  enable_http2 = true
  comment      = "terraform acceptance test"
}
`, stubURL)
}

func TestBuildServerConfigPolicyPluginCfgData(t *testing.T) {
	tests := []struct {
		name     string
		config   S3MinioServerConfigPolicyPlugin
		contains []string
	}{
		{
			name: "all fields",
			config: S3MinioServerConfigPolicyPlugin{
				URL:         "http://opa:8181/v1/data/httpapi/authz",
				AuthToken:   "stub token",
				EnableHTTP2: true,
				Comment:     "probe value",
			},
			contains: []string{
				"url=http://opa:8181/v1/data/httpapi/authz",
				`auth_token="stub token"`,
				"enable_http2=on",
				`comment="probe value"`,
			},
		},
		{
			name:   "empty optionals are sent as empty keys",
			config: S3MinioServerConfigPolicyPlugin{URL: "http://opa:8181"},
			contains: []string{
				"url=http://opa:8181",
				"auth_token=",
				"enable_http2=off",
				"comment=",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildServerConfigPolicyPluginCfgData(&tc.config)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in config data, got: %s", want, got)
				}
			}
		})
	}
}

func TestPolicyPluginRunningConfigMatchesState(t *testing.T) {
	// The server prints the stored keys and redacts a non-empty auth_token,
	// so the response carries no auth_token key at all.
	cfgMap := map[string]string{
		"url":          "http://opa:8181",
		"enable_http2": "on",
		"comment":      "probe value",
	}
	config := &S3MinioServerConfigPolicyPlugin{
		URL:         "http://opa:8181",
		AuthToken:   "stub token",
		EnableHTTP2: true,
		Comment:     "probe value",
	}
	if !policyPluginRunningConfigMatchesState(cfgMap, config) {
		t.Errorf("running config matches the state but was reported stale")
	}

	stale := map[string]string{
		"url":          "http://other:8181",
		"enable_http2": "off",
		"comment":      "",
	}
	if policyPluginRunningConfigMatchesState(stale, config) {
		t.Errorf("stale running config was reported as matching")
	}

	// An empty auth_token comes back as auth_token= and must agree with an
	// empty configuration.
	emptyTokenCfgMap := map[string]string{
		"url":          "http://opa:8181",
		"auth_token":   "",
		"enable_http2": "off",
		"comment":      "",
	}
	emptyTokenConfig := &S3MinioServerConfigPolicyPlugin{URL: "http://opa:8181"}
	if !policyPluginRunningConfigMatchesState(emptyTokenCfgMap, emptyTokenConfig) {
		t.Errorf("empty stored auth_token did not match an empty configuration")
	}
}
