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

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
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

// MinIO accepts a key the scanner subsystem does not have and glues it onto
// the value written before it. An older provider wrote the excess thresholds
// as excess_versions and excess_folders, so the checks below assert that the
// server stores them as alert_excess_versions and alert_excess_folders and
// keeps cycle intact. See issue #1322.

func TestAccMinioServerConfigScanner_excessThresholds(t *testing.T) {
	resourceName := "minio_server_config_scanner.test"
	// Values differ from the server defaults (100 and 50000), so the stored
	// check passes only if the thresholds reached the alert_excess_* keys.
	config := `
resource "minio_server_config_scanner" "test" {
  speed           = "slow"
  delay           = "10"
  max_wait        = "15s"
  cycle           = "1m"
  excess_versions = "10"
  excess_folders  = "25"
}`

	// Sequential on purpose: the server check restarts the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "cycle", "1m"),
					resource.TestCheckResourceAttr(resourceName, "excess_versions", "10"),
					resource.TestCheckResourceAttr(resourceName, "excess_folders", "25"),
					testAccCheckServerStoredTarget("scanner", "", map[string]string{
						"speed":                 "slow",
						"delay":                 "10",
						"max_wait":              "15s",
						"cycle":                 "1m",
						"alert_excess_versions": "10",
						"alert_excess_folders":  "25",
					}),
				),
			},
			{Config: config, PlanOnly: true},
		},
	})
}

// A scanner configuration written by an older provider stores the excess
// thresholds glued onto cycle. The read reports the server value, so the
// refreshed state must show the glued value, the plan must show the cycle
// diff, and the next apply must rewrite a clean cycle and move the
// thresholds to the alert_excess_* keys.
func TestAccMinioServerConfigScanner_excessThresholdsCorruptedByOlderProvider(t *testing.T) {
	resourceName := "minio_server_config_scanner.test"
	config := `
resource "minio_server_config_scanner" "test" {
  speed           = "slow"
  delay           = "10"
  max_wait        = "15s"
  cycle           = "1m"
  excess_versions = "100"
  excess_folders  = "50000"
}`

	// Sequential on purpose: the server checks restart the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "cycle", "1m"),
				),
			},
			{
				PreConfig: func() {
					admin := testAccClient().S3Admin
					// Reset the subsystem first so the server looks like one
					// an older provider wrote: alert_excess_* at their
					// defaults and the excess thresholds glued onto cycle.
					if _, err := admin.DelConfigKV(context.Background(), "scanner"); err != nil {
						t.Fatalf("resetting scanner before the corruption: %v", err)
					}
					if err := testAccRestartPrimaryServer(); err != nil {
						t.Fatalf("restarting the server: %v", err)
					}
					old := "scanner speed=slow delay=10 max_wait=15s cycle=1m excess_versions=100 excess_folders=50000"
					if _, err := admin.SetConfigKV(context.Background(), old); err != nil {
						t.Fatalf("writing scanner the way the older provider did: %v", err)
					}
					if err := testAccRestartPrimaryServer(); err != nil {
						t.Fatalf("restarting the server: %v", err)
					}
				},
				// The refreshed state must show the corruption, and the cycle
				// diff it causes is what the next step applies.
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check:              resource.TestCheckResourceAttr(resourceName, "cycle", "1m excess_versions=100 excess_folders=50000"),
			},
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "cycle", "1m"),
					resource.TestCheckResourceAttr(resourceName, "excess_versions", "100"),
					resource.TestCheckResourceAttr(resourceName, "excess_folders", "50000"),
					testAccCheckServerStoredTarget("scanner", "", map[string]string{
						"cycle":                 "1m",
						"alert_excess_versions": "100",
						"alert_excess_folders":  "50000",
					}),
				),
			},
			{Config: config, PlanOnly: true},
		},
	})
}

// The upgrade path for a cycle set outside Terraform: the old provider wrote
// speed, delay, max_wait, cycle and then the excess thresholds, so the
// unknown keys were glued onto the value written before them, cycle. cycle is
// Optional and Computed, so the glued value never shows a normal diff when
// the configuration does not set it. The CustomizeDiff must force the cycle
// diff so the next apply writes a clean value instead of gluing the old keys
// on again. The thresholds equal the server defaults, so the forced cycle
// diff is the only one the apply acts on.
func TestAccMinioServerConfigScanner_corruptedCycleWithoutCycleInConfig(t *testing.T) {
	resourceName := "minio_server_config_scanner.test"
	config := `
resource "minio_server_config_scanner" "test" {
  speed           = "slow"
  delay           = "10"
  max_wait        = "15s"
  excess_versions = "100"
  excess_folders  = "50000"
}`

	// Sequential on purpose: the server checks restart the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				// The server prints no cycle key for a scanner configuration
				// that does not set one, so no cycle attribute lands in the
				// state.
				Config: config,
				Check:  resource.TestCheckNoResourceAttr(resourceName, "cycle"),
			},
			{
				PreConfig: func() {
					admin := testAccClient().S3Admin
					// Reset the subsystem first so the server looks like one
					// an older provider wrote: alert_excess_* at their
					// defaults and the excess thresholds glued onto cycle.
					if _, err := admin.DelConfigKV(context.Background(), "scanner"); err != nil {
						t.Fatalf("resetting scanner before the corruption: %v", err)
					}
					if err := testAccRestartPrimaryServer(); err != nil {
						t.Fatalf("restarting the server: %v", err)
					}
					old := "scanner speed=slow delay=10 max_wait=15s cycle=1m excess_versions=100 excess_folders=50000"
					if _, err := admin.SetConfigKV(context.Background(), old); err != nil {
						t.Fatalf("writing scanner the way the older provider did: %v", err)
					}
					if err := testAccRestartPrimaryServer(); err != nil {
						t.Fatalf("restarting the server: %v", err)
					}
				},
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "cycle", "1m"),
					testAccCheckServerStoredTarget("scanner", "", map[string]string{
						"cycle":                 "1m",
						"alert_excess_versions": "100",
						"alert_excess_folders":  "50000",
					}),
				),
			},
			{Config: config, PlanOnly: true},
		},
	})
}

// The upgrade path when the old provider never wrote cycle: it wrote speed,
// delay, max_wait and then the excess thresholds, so the unknown keys were
// glued onto the value written before them, max_wait. max_wait is Optional
// and Computed, so when the configuration does not set it, the glued value
// never shows a normal diff and the CustomizeDiff must force it so the next
// apply writes a clean value. The thresholds equal the server defaults, so
// the forced max_wait diff is the only one the apply acts on.
func TestAccMinioServerConfigScanner_corruptedMaxWaitWithoutMaxWaitInConfig(t *testing.T) {
	resourceName := "minio_server_config_scanner.test"
	config := `
resource "minio_server_config_scanner" "test" {
  speed           = "slow"
  delay           = "10"
  excess_versions = "100"
  excess_folders  = "50000"
}`

	// Sequential on purpose: the server checks restart the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				// The server prints no max_wait key for a scanner
				// configuration that does not set one, so no max_wait
				// attribute lands in the state.
				Config: config,
				Check:  resource.TestCheckNoResourceAttr(resourceName, "max_wait"),
			},
			{
				PreConfig: func() {
					admin := testAccClient().S3Admin
					// Reset the subsystem first so the server looks like one
					// an older provider wrote: alert_excess_* at their
					// defaults and the excess thresholds glued onto max_wait,
					// the value written before them.
					if _, err := admin.DelConfigKV(context.Background(), "scanner"); err != nil {
						t.Fatalf("resetting scanner before the corruption: %v", err)
					}
					if err := testAccRestartPrimaryServer(); err != nil {
						t.Fatalf("restarting the server: %v", err)
					}
					old := "scanner speed=slow delay=10 max_wait=15s excess_versions=100 excess_folders=50000"
					if _, err := admin.SetConfigKV(context.Background(), old); err != nil {
						t.Fatalf("writing scanner the way the older provider did: %v", err)
					}
					if err := testAccRestartPrimaryServer(); err != nil {
						t.Fatalf("restarting the server: %v", err)
					}
				},
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "max_wait", "15s"),
					testAccCheckServerStoredTarget("scanner", "", map[string]string{
						"max_wait":              "15s",
						"alert_excess_versions": "100",
						"alert_excess_folders":  "50000",
					}),
				),
			},
			{Config: config, PlanOnly: true},
		},
	})
}

func TestBuildScannerCfgUsesAlertExcessKeys(t *testing.T) {
	d := schema.TestResourceDataRaw(t, resourceMinioServerConfigScanner().Schema, map[string]interface{}{
		"speed":           "slow",
		"delay":           "10",
		"max_wait":        "15s",
		"cycle":           "1m",
		"excess_versions": "10",
		"excess_folders":  "25",
	})

	got := buildScannerCfg(d)
	for _, want := range []string{"speed=slow", "delay=10", "max_wait=15s", "cycle=1m", "alert_excess_versions=10", "alert_excess_folders=25"} {
		if !strings.Contains(got, want) {
			t.Errorf("buildScannerCfg() = %q, want it to contain %q", got, want)
		}
	}
	for _, bad := range []string{" excess_versions=", " excess_folders="} {
		if strings.Contains(got, bad) {
			t.Errorf("buildScannerCfg() = %q contains %q, a key the scanner subsystem does not have", got, bad)
		}
	}
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

func TestServerConfigEtcdRunningConfigMatchesState(t *testing.T) {
	stateWith := func(cert, certKey string) *schema.ResourceData {
		return schema.TestResourceDataRaw(t, resourceMinioServerConfigEtcd().Schema, map[string]interface{}{
			"endpoints":       "http://etcd:2379",
			"client_cert":     cert,
			"client_cert_key": certKey,
		})
	}

	// The maps below are responses measured from the test server, not the
	// output of the etcd builder.
	t.Run("pre-restart response with cleared certs does not match", func(t *testing.T) {
		if etcdRunningConfigMatchesState(stateWith("/tmp/client.crt", "/tmp/client.key"), map[string]string{
			"endpoints": "http://etcd:2379",
		}) {
			t.Errorf("expected no match for the pre-restart response")
		}
	})

	t.Run("post-restart response matches", func(t *testing.T) {
		if !etcdRunningConfigMatchesState(stateWith("/tmp/client.crt", "/tmp/client.key"), map[string]string{
			"endpoints":       "http://etcd:2379",
			"client_cert":     "/tmp/client.crt",
			"client_cert_key": "/tmp/client.key",
		}) {
			t.Errorf("expected the post-restart response to match")
		}
	})

	t.Run("stale cert path does not match a cleared state", func(t *testing.T) {
		if etcdRunningConfigMatchesState(stateWith("", ""), map[string]string{
			"endpoints":   "http://etcd:2379",
			"client_cert": "/tmp/client.crt",
		}) {
			t.Errorf("expected no match while the server still reports the removed cert")
		}
	})

	t.Run("cleared certs match the cleared response", func(t *testing.T) {
		if !etcdRunningConfigMatchesState(stateWith("", ""), map[string]string{
			"endpoints":       "http://etcd:2379",
			"client_cert":     "",
			"client_cert_key": "",
		}) {
			t.Errorf("expected the cleared post-restart response to match")
		}
	})

	t.Run("stale endpoints do not match", func(t *testing.T) {
		if etcdRunningConfigMatchesState(stateWith("/tmp/client.crt", "/tmp/client.key"), map[string]string{
			"endpoints":       "http://other-etcd:2379",
			"client_cert":     "/tmp/client.crt",
			"client_cert_key": "/tmp/client.key",
		}) {
			t.Errorf("expected no match while the server reports different endpoints")
		}
	})

	t.Run("key without cert matches the key-only response", func(t *testing.T) {
		if !etcdRunningConfigMatchesState(stateWith("", "/tmp/client.key"), map[string]string{
			"endpoints":       "http://etcd:2379",
			"client_cert":     "",
			"client_cert_key": "/tmp/client.key",
		}) {
			t.Errorf("expected the key-only response to match")
		}
	})
}

func TestServerConfigEtcdStateUpgradeV0(t *testing.T) {
	res := resourceMinioServerConfigEtcd()
	if res.SchemaVersion != 1 {
		t.Errorf("SchemaVersion is %d, want 1", res.SchemaVersion)
	}
	if len(res.StateUpgraders) != 1 || res.StateUpgraders[0].Version != 0 || res.StateUpgraders[0].Upgrade == nil {
		t.Errorf("expected exactly one v0 state upgrader, got %+v", res.StateUpgraders)
	}

	rawState := map[string]interface{}{
		"endpoints":        "http://etcd:2379",
		"client_cert":      "",
		"restart_required": true,
	}

	state, err := resourceMinioServerConfigEtcdUpgradeV0(context.Background(), rawState, nil)
	if err != nil {
		t.Fatalf("upgrading the v0 state: %v", err)
	}
	if state["restart_required"] != false {
		t.Errorf("restart_required after the upgrade is %v, want false", state["restart_required"])
	}
	if state["endpoints"] != "http://etcd:2379" {
		t.Errorf("endpoints after the upgrade is %v, want the stored value", state["endpoints"])
	}
}

// TestAccMinioServerConfigEtcd_basic requires TF_ACC_ETCD_ENDPOINTS to be set.
// No etcd is available in the standard CI environment, so this is skipped by default.
func TestAccMinioServerConfigEtcd_basic(t *testing.T) {
	endpoints := os.Getenv("TF_ACC_ETCD_ENDPOINTS")
	if endpoints == "" {
		t.Skip("TF_ACC_ETCD_ENDPOINTS not set — skipping etcd config acceptance test")
	}

	resourceName := "minio_server_config_etcd.test"

	// Sequential on purpose: the destroy check restarts the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		// The delete only queues the reset: the destroy check restarts to
		// apply it and leaves the server out of etcd mode for whatever runs
		// next.
		CheckDestroy: testAccCheckServerEtcdParams(map[string]string{"endpoints": ""}),
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_server_config_etcd" "test" {
  endpoints = "` + endpoints + `"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "endpoints", endpoints),
					// The restart here is what makes the import step below
					// meaningful: get-config-kv serves the pre-restart
					// section while the config is pending, so an import
					// before the restart would read a section without the
					// endpoints.
					testAccCheckServerEtcdParams(map[string]string{"endpoints": endpoints}),
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

// TestAccMinioServerConfigEtcd_clearClientCert also requires TF_ACC_ETCD_ENDPOINTS.
// It verifies that removing client_cert and client_cert_key from the
// configuration sends them as empty key= so MinIO clears them (the server
// merges set-config-kv, so a key left out keeps its old value), that the plan
// converges once the restart is applied, and that the read gate holds the
// state against the pre-restart response while a restart is pending.
func TestAccMinioServerConfigEtcd_clearClientCert(t *testing.T) {
	endpoints := os.Getenv("TF_ACC_ETCD_ENDPOINTS")
	if endpoints == "" {
		t.Skip("TF_ACC_ETCD_ENDPOINTS not set — skipping etcd config acceptance test")
	}

	resourceName := "minio_server_config_etcd.test"

	// Sequential on purpose: the server checks restart the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		// The delete only queues the reset: DelConfigKV needs a restart too.
		// Restarting here applies it and leaves the primary server out of etcd
		// mode for the tests that run after this one.
		CheckDestroy: testAccCheckServerEtcdParams(map[string]string{"endpoints": ""}),
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_server_config_etcd" "test" {
  endpoints       = "` + endpoints + `"
  client_cert     = "/tmp/client.crt"
  client_cert_key = "/tmp/client.key"
  comment         = "clear test"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "client_cert", "/tmp/client.crt"),
					// comment is in the map so the gate's comment check is
					// exercised: if get-config-kv ever stops returning it,
					// the gate never opens and this step's plan fails.
					testAccCheckServerEtcdParams(map[string]string{
						"client_cert":     "/tmp/client.crt",
						"client_cert_key": "/tmp/client.key",
						"comment":         "clear test",
					}),
				),
			},
			{
				Config: `
resource "minio_server_config_etcd" "test" {
  endpoints = "` + endpoints + `"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "client_cert", ""),
					testAccCheckServerEtcdParams(map[string]string{
						"client_cert":     "",
						"client_cert_key": "",
					}),
				),
			},
			{
				// Re-add the pair and do not restart. The framework's
				// refresh-then-plan after this step is what exercises the
				// read gate: without the gate the read writes the empty
				// pre-restart value and the plan never converges until the
				// restart. The Check only pins the apply result (the flag
				// comes from SetConfigKV, not from the gate).
				Config: `
resource "minio_server_config_etcd" "test" {
  endpoints       = "` + endpoints + `"
  client_cert     = "/tmp/client.crt"
  client_cert_key = "/tmp/client.key"
}`,
				Check: resource.TestCheckResourceAttr(resourceName, "restart_required", "true"),
			},
		},
	})
}

// testAccCheckServerEtcdParams restarts the primary test server and checks
// that get-config-kv reports the etcd subsystem with the given key=value
// pairs. A cleared key is checked against "".
func testAccCheckServerEtcdParams(want map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if err := testAccRestartPrimaryServer(); err != nil {
			return err
		}

		client := testAccClient()
		data, err := client.S3Admin.GetConfigKV(context.Background(), "etcd")
		if err != nil {
			return fmt.Errorf("reading the etcd config from the server after the restart: %w", err)
		}
		// Same line-by-line read as the resource: a MINIO_ETCD_* environment
		// variable makes get-config-kv print a "# MINIO_ETCD_..." line first.
		stored, _ := notifyTargetConfigParams(string(data), "etcd")
		for key, value := range want {
			got, ok := stored[key]
			if !ok {
				return fmt.Errorf("the etcd config stored on the server has no %q key; response: %s", key, string(data))
			}
			if got != value {
				return fmt.Errorf("%q of the etcd config stored on the server is %q, want %q", key, got, value)
			}
		}
		return nil
	}
}

// The compression subsystem is a singleton: every test writes the same
// server config, so these tests must stay sequential resource.Test calls and
// must not run in parallel with each other.
func TestAccMinioServerConfigCompression_basic(t *testing.T) {
	resourceName := "minio_server_config_compression.test"

	// Sequential on purpose: the server checks restart the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		// The subsystem cannot be deleted, only reset: after the destroy the
		// server is back to the compression defaults seeded by MinIO.
		CheckDestroy: testAccCheckServerStoredTarget("compression", "", map[string]string{
			"enable":           "off",
			"allow_encryption": "off",
			"extensions":       ".txt,.log,.csv,.json,.tar,.xml,.bin",
			"mime_types":       "text/*,application/json,application/xml,binary/octet-stream",
		}),
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_server_config_compression" "test" {
  enable             = true
  allow_encryption   = true
  extensions         = [".txt", ".log"]
  mime_types         = ["application/json", "text/plain"]
  comment            = "terraform acceptance test"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enable", "true"),
					resource.TestCheckResourceAttr(resourceName, "allow_encryption", "true"),
					resource.TestCheckResourceAttr(resourceName, "extensions.#", "2"),
					resource.TestCheckResourceAttr(resourceName, "extensions.0", ".txt"),
					resource.TestCheckResourceAttr(resourceName, "extensions.1", ".log"),
					resource.TestCheckResourceAttr(resourceName, "mime_types.#", "2"),
					resource.TestCheckResourceAttr(resourceName, "mime_types.0", "application/json"),
					resource.TestCheckResourceAttr(resourceName, "mime_types.1", "text/plain"),
					resource.TestCheckResourceAttr(resourceName, "comment", "terraform acceptance test"),
					// The server prints enable only when it is off, so with
					// enable = true the stored config carries no enable key.
					testAccCheckServerStoredTarget("compression", "", map[string]string{
						"allow_encryption": "on",
						"extensions":       ".txt,.log",
						"mime_types":       "application/json,text/plain",
						"comment":          "terraform acceptance test",
					}),
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

func TestAccMinioServerConfigCompression_update(t *testing.T) {
	resourceName := "minio_server_config_compression.test"

	// Sequential on purpose: the compression subsystem is a shared singleton
	// and the server checks restart the primary test server.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_server_config_compression" "test" {
  enable             = false
  allow_encryption   = true
  extensions         = [".txt", ".log"]
  mime_types         = ["text/*"]
  comment            = "update step one"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
					testAccCheckServerStoredTarget("compression", "", map[string]string{
						"enable":           "off",
						"allow_encryption": "on",
						"extensions":       ".txt,.log",
						"mime_types":       "text/*",
						"comment":          "update step one",
					}),
				),
			},
			{
				Config: `
resource "minio_server_config_compression" "test" {
  enable             = true
  allow_encryption   = false
  extensions         = [".json"]
  mime_types         = ["application/xml"]
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enable", "true"),
					resource.TestCheckResourceAttr(resourceName, "allow_encryption", "false"),
					resource.TestCheckResourceAttr(resourceName, "extensions.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "extensions.0", ".json"),
					resource.TestCheckResourceAttr(resourceName, "comment", ""),
					testAccCheckServerStoredTarget("compression", "", map[string]string{
						"allow_encryption": "off",
						"extensions":       ".json",
						"mime_types":       "application/xml",
						"comment":          "",
					}),
				),
			},
			{
				Config: `
resource "minio_server_config_compression" "test" {
  enable     = true
  extensions = []
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "extensions.#", "0"),
					testAccCheckServerStoredTarget("compression", "", map[string]string{
						"extensions": "",
					}),
				),
			},
		},
	})
}

// A comment set outside Terraform must not survive a create that leaves the
// field empty: the resource owns the singleton, so the create clears it.
func TestAccMinioServerConfigCompression_existingCommentCleared(t *testing.T) {
	resourceName := "minio_server_config_compression.test"

	// Sequential on purpose: the compression subsystem is a shared singleton
	// and the checks restart the primary test server.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				PreConfig: func() { testAccSeedCompressionComment(t) },
				Config: `
resource "minio_server_config_compression" "test" {
  enable = true
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "comment", ""),
					testAccCheckServerStoredTarget("compression", "", map[string]string{
						"comment": "",
					}),
				),
			},
			{
				// The create cleared the seeded comment, so the next plan is
				// empty.
				Config: `
resource "minio_server_config_compression" "test" {
  enable = true
}`,
				PlanOnly: true,
			},
		},
	})
}

func testAccSeedCompressionComment(t *testing.T) {
	t.Helper()
	if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), "compression comment=x"); err != nil {
		t.Fatalf("seeding the compression comment outside Terraform: %v", err)
	}
	if err := testAccRestartPrimaryServer(); err != nil {
		t.Fatalf("restarting the server: %v", err)
	}
}

// An explicit empty list at create must clear the server's list on the first
// apply: the SDK sees no diff for it (no old value and an empty new value are
// equal), so the clear cannot be driven by HasChange.
func TestAccMinioServerConfigCompression_emptyListOnCreate(t *testing.T) {
	resourceName := "minio_server_config_compression.test"

	// Sequential on purpose: the compression subsystem is a shared singleton.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_server_config_compression" "test" {
  enable     = true
  extensions = []
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "extensions.#", "0"),
					testAccCheckServerStoredTarget("compression", "", map[string]string{
						"extensions": "",
					}),
				),
			},
			{
				// The create cleared the server's default extensions, so the
				// next plan is empty.
				Config: `
resource "minio_server_config_compression" "test" {
  enable     = true
  extensions = []
}`,
				PlanOnly: true,
			},
		},
	})
}

func TestBuildServerConfigCompressionCfgData(t *testing.T) {
	d := schema.TestResourceDataRaw(t, resourceMinioServerConfigCompression().Schema, map[string]interface{}{})

	tests := []struct {
		name     string
		config   S3MinioServerConfigCompression
		contains []string
		absent   []string
	}{
		{
			name: "all fields",
			config: S3MinioServerConfigCompression{
				Enable:          true,
				Extensions:      []string{".txt", ".log"},
				MimeTypes:       []string{"application/json", "text/plain"},
				AllowEncryption: true,
				Comment:         "probe value",
			},
			contains: []string{
				"enable=on",
				"allow_encryption=on",
				"extensions=.txt,.log",
				"mime_types=application/json,text/plain",
				`comment="probe value"`,
			},
		},
		{
			name: "disabled with no optional fields",
			config: S3MinioServerConfigCompression{
				Enable: false,
			},
			contains: []string{
				"enable=off",
				"allow_encryption=off",
				"comment=",
			},
			absent: []string{
				"extensions=",
				"mime_types=",
			},
		},
		{
			name: "disabled with values",
			config: S3MinioServerConfigCompression{
				Enable:          false,
				Extensions:      []string{".csv"},
				MimeTypes:       []string{"text/*"},
				AllowEncryption: false,
			},
			contains: []string{
				"enable=off",
				"allow_encryption=off",
				"extensions=.csv",
				"mime_types=text/*",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildServerConfigCompressionCfgData(d, &tc.config)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in config data, got: %s", want, got)
				}
			}
			for _, unwanted := range tc.absent {
				if strings.Contains(got, unwanted) {
					t.Errorf("unexpected %q in config data, got: %s", unwanted, got)
				}
			}
		})
	}
}

func TestValidateCompressionListItem(t *testing.T) {
	valid := []string{".txt", "application/json", "text/*"}
	for _, v := range valid {
		if _, errs := validateCompressionListItem(v, "extensions"); len(errs) != 0 {
			t.Errorf("validateCompressionListItem(%q): unexpected error: %v", v, errs)
		}
	}

	invalid := []string{"", ".txt,.log", "bad\nname", `"leading`}
	for _, v := range invalid {
		if _, errs := validateCompressionListItem(v, "extensions"); len(errs) == 0 {
			t.Errorf("validateCompressionListItem(%q): expected an error", v)
		}
	}
}

func TestConfiguredEmptyList(t *testing.T) {
	objectType := cty.Object(map[string]cty.Type{
		"extensions": cty.List(cty.String),
		"mime_types": cty.List(cty.String),
	})

	tests := []struct {
		name string
		raw  cty.Value
		key  string
		want bool
	}{
		{"nil raw config carries none", cty.NilVal, "extensions", false},
		{"null raw config", cty.NullVal(objectType), "extensions", false},
		{"explicit empty list", cty.ObjectVal(map[string]cty.Value{
			"extensions": cty.ListValEmpty(cty.String),
			"mime_types": cty.ListValEmpty(cty.String),
		}), "extensions", true},
		{"attribute absent from the configuration", cty.ObjectVal(map[string]cty.Value{
			"extensions": cty.ListValEmpty(cty.String),
		}), "mime_types", false},
		{"non-empty list", cty.ObjectVal(map[string]cty.Value{
			"extensions": cty.ListVal([]cty.Value{cty.StringVal(".txt")}),
		}), "extensions", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := configuredEmptyList(tc.raw, tc.key); got != tc.want {
				t.Errorf("configuredEmptyList(%v, %q) = %v, want %v", tc.raw, tc.key, got, tc.want)
			}
		})
	}
}

func TestAccMinioServerConfigBrowser_basic(t *testing.T) {
	resourceName := "minio_server_config_browser.test"

	// A CSP policy with spaces and single quotes: it must go through
	// buildConfigParam quoted, and the server must store it verbatim.
	csp := "default-src 'self'; img-src 'self' https://cdn.example.com"

	// Sequential on purpose: the browser subsystem is a shared singleton
	// and the server checks restart the primary test server.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		// The subsystem cannot be deleted, only reset: after the destroy the
		// server is back to the browser defaults seeded by MinIO.
		CheckDestroy: testAccCheckServerStoredTarget("browser", "", map[string]string{
			"csp_policy":              browserDefaultCspPolicy,
			"hsts_seconds":            "0",
			"hsts_include_subdomains": "off",
			"hsts_preload":            "off",
			"referrer_policy":         "strict-origin-when-cross-origin",
		}),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "minio_server_config_browser" "test" {
  csp_policy              = %q
  hsts_seconds            = 31536000
  hsts_include_subdomains = true
  hsts_preload            = true
  referrer_policy         = "strict-origin-when-cross-origin"
}`, csp),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "csp_policy", csp),
					resource.TestCheckResourceAttr(resourceName, "hsts_seconds", "31536000"),
					resource.TestCheckResourceAttr(resourceName, "hsts_include_subdomains", "true"),
					resource.TestCheckResourceAttr(resourceName, "hsts_preload", "true"),
					resource.TestCheckResourceAttr(resourceName, "referrer_policy", "strict-origin-when-cross-origin"),
					testAccCheckServerStoredTarget("browser", "", map[string]string{
						"csp_policy":              csp,
						"hsts_seconds":            "31536000",
						"hsts_include_subdomains": "on",
						"hsts_preload":            "on",
						"referrer_policy":         "strict-origin-when-cross-origin",
					}),
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

func TestAccMinioServerConfigBrowser_update(t *testing.T) {
	resourceName := "minio_server_config_browser.test"

	csp := "default-src 'self'; img-src 'self' https://cdn.example.com"

	// Sequential on purpose: the browser subsystem is a shared singleton
	// and the server checks restart the primary test server.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_server_config_browser" "test" {
  hsts_seconds            = 31536000
  hsts_include_subdomains = true
  hsts_preload            = true
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "hsts_include_subdomains", "true"),
					resource.TestCheckResourceAttr(resourceName, "hsts_preload", "true"),
					testAccCheckServerStoredTarget("browser", "", map[string]string{
						"csp_policy":              "",
						"hsts_seconds":            "31536000",
						"hsts_include_subdomains": "on",
						"hsts_preload":            "on",
					}),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "minio_server_config_browser" "test" {
  csp_policy      = %q
  hsts_seconds    = 3600
  referrer_policy = "no-referrer"
}`, csp),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "csp_policy", csp),
					resource.TestCheckResourceAttr(resourceName, "hsts_seconds", "3600"),
					resource.TestCheckResourceAttr(resourceName, "hsts_include_subdomains", "false"),
					resource.TestCheckResourceAttr(resourceName, "hsts_preload", "false"),
					resource.TestCheckResourceAttr(resourceName, "referrer_policy", "no-referrer"),
					testAccCheckServerStoredTarget("browser", "", map[string]string{
						"csp_policy":              csp,
						"hsts_seconds":            "3600",
						"hsts_include_subdomains": "off",
						"hsts_preload":            "off",
						"referrer_policy":         "no-referrer",
					}),
				),
			},
		},
	})
}

// A csp_policy set outside Terraform must not survive a create that leaves
// the field empty: the resource owns the singleton, so the create clears it
// back to the server's built-in default.
func TestAccMinioServerConfigBrowser_existingCspPolicyCleared(t *testing.T) {
	resourceName := "minio_server_config_browser.test"

	// Sequential on purpose: the checks restart the primary test server.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy: testAccCheckServerStoredTarget("browser", "", map[string]string{
			"csp_policy":              browserDefaultCspPolicy,
			"hsts_seconds":            "0",
			"hsts_include_subdomains": "off",
			"hsts_preload":            "off",
			"referrer_policy":         "strict-origin-when-cross-origin",
		}),
		Steps: []resource.TestStep{
			{
				PreConfig: func() { testAccSeedBrowserCspPolicy(t) },
				Config: `
resource "minio_server_config_browser" "test" {
  hsts_seconds = 3600
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "csp_policy", ""),
					testAccCheckServerStoredTarget("browser", "", map[string]string{
						"csp_policy":   "",
						"hsts_seconds": "3600",
					}),
				),
			},
			{
				// The create cleared the seeded policy, so the next plan is
				// empty.
				Config: `
resource "minio_server_config_browser" "test" {
  hsts_seconds = 3600
}`,
				PlanOnly: true,
			},
		},
	})
}

func testAccSeedBrowserCspPolicy(t *testing.T) {
	t.Helper()
	if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), "browser csp_policy=seeded-by-test"); err != nil {
		t.Fatalf("seeding the csp_policy outside Terraform: %v", err)
	}
	if err := testAccRestartPrimaryServer(); err != nil {
		t.Fatalf("restarting the server: %v", err)
	}
}

// browserDefaultCspPolicy is the Content-Security-Policy the server prints
// for the browser subsystem after a reset: the default from browser.DefaultKVS
// in MinIO, verbatim (the two spaces before connect-src are in the default).
const browserDefaultCspPolicy = "default-src 'self' 'unsafe-eval' 'unsafe-inline'; script-src 'self' https://unpkg.com;  connect-src 'self' https://unpkg.com;"

func TestBuildServerConfigBrowserCfgData(t *testing.T) {
	tests := []struct {
		name     string
		config   S3MinioServerConfigBrowser
		contains []string
		absent   []string
	}{
		{
			name: "all fields",
			config: S3MinioServerConfigBrowser{
				CspPolicy:             "default-src 'self'; img-src 'self' https://cdn.example.com",
				HstsSeconds:           31536000,
				HstsIncludeSubdomains: true,
				HstsPreload:           true,
				ReferrerPolicy:        "strict-origin",
			},
			contains: []string{
				`csp_policy="default-src 'self'; img-src 'self' https://cdn.example.com"`,
				"hsts_seconds=31536000",
				"hsts_include_subdomains=on",
				"hsts_preload=on",
				"referrer_policy=strict-origin",
			},
			absent: []string{
				"hsts_include_subdomains=off",
				"hsts_preload=off",
			},
		},
		{
			name:   "all fields empty",
			config: S3MinioServerConfigBrowser{},
			contains: []string{
				"csp_policy=",
				"hsts_seconds=0",
				"hsts_include_subdomains=off",
				"hsts_preload=off",
				"referrer_policy=",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildServerConfigBrowserCfgData(&tc.config)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in config data, got: %s", want, got)
				}
			}
			for _, unwanted := range tc.absent {
				if strings.Contains(got, unwanted) {
					t.Errorf("unexpected %q in config data, got: %s", unwanted, got)
				}
			}
		})
	}
}

func TestAccMinioServerConfigSite_basic(t *testing.T) {
	resourceName := "minio_server_config_site.test"

	// Sequential on purpose: the site subsystem is a shared singleton and the
	// server checks restart the primary test server.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		// The subsystem cannot be deleted, only reset: after the destroy the
		// server is back to the site and region defaults.
		CheckDestroy: resource.ComposeTestCheckFunc(
			testAccCheckServerStoredTarget("site", "", map[string]string{
				"name":   "",
				"region": "",
			}),
			testAccCheckServerStoredTarget("region", "", map[string]string{
				"name": "",
			}),
		),
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_server_config_site" "test" {
  name    = "tf-site"
  region  = "us-east-1"
  comment = "managed by terraform"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "tf-site"),
					resource.TestCheckResourceAttr(resourceName, "region", "us-east-1"),
					resource.TestCheckResourceAttr(resourceName, "comment", "managed by terraform"),
					// The site is stored under the site subsystem.
					testAccCheckServerStoredTarget("site", "", map[string]string{
						"name":    "tf-site",
						"region":  "us-east-1",
						"comment": "managed by terraform",
					}),
					// Setting the site does not write to the legacy region
					// subsystem: it stays at its default.
					testAccCheckServerStoredTarget("region", "", map[string]string{
						"name": "",
					}),
				),
			},
			{
				Config: `
resource "minio_server_config_site" "test" {
  name   = "tf-site"
  region = "eu-west-1"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "tf-site"),
					resource.TestCheckResourceAttr(resourceName, "region", "eu-west-1"),
					resource.TestCheckResourceAttr(resourceName, "comment", ""),
					testAccCheckServerStoredTarget("site", "", map[string]string{
						"name":    "tf-site",
						"region":  "eu-west-1",
						"comment": "",
					}),
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

// A site name and region set outside Terraform must not survive a create that
// leaves the fields empty: the resource owns the singleton, so the create
// overwrites the name and clears the rest.
func TestAccMinioServerConfigSite_existingValuesCleared(t *testing.T) {
	resourceName := "minio_server_config_site.test"

	// Sequential on purpose: the site subsystem is a shared singleton and the
	// checks restart the primary test server.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t); testAccCleanupSite(t) },
		ProviderFactories: testAccProviders,
		// The subsystem cannot be deleted, only reset: after the destroy the
		// server is back to the site and region defaults.
		CheckDestroy: resource.ComposeTestCheckFunc(
			testAccCheckServerStoredTarget("site", "", map[string]string{
				"name":   "",
				"region": "",
			}),
			testAccCheckServerStoredTarget("region", "", map[string]string{
				"name": "",
			}),
		),
		Steps: []resource.TestStep{
			{
				PreConfig: func() { testAccSeedSiteOutsideTerraform(t) },
				Config: `
resource "minio_server_config_site" "test" {
  name = "tf-site"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "tf-site"),
					// region and comment are absent from the flat state, not
					// empty: an Optional string the configuration never set has no
					// key to check.
					resource.TestCheckNoResourceAttr(resourceName, "region"),
					resource.TestCheckNoResourceAttr(resourceName, "comment"),
					testAccCheckServerStoredTarget("site", "", map[string]string{
						"name":    "tf-site",
						"region":  "",
						"comment": "",
					}),
				),
			},
			{
				// The create cleared the seeded values, so the next plan is
				// empty.
				Config: `
resource "minio_server_config_site" "test" {
  name = "tf-site"
}`,
				PlanOnly: true,
			},
		},
	})
}

func testAccSeedSiteOutsideTerraform(t *testing.T) {
	t.Helper()
	if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), "site name=seeded-site region=seeded-region"); err != nil {
		t.Fatalf("seeding the site outside Terraform: %v", err)
	}
	if err := testAccRestartPrimaryServer(); err != nil {
		t.Fatalf("restarting the server: %v", err)
	}
}

// testAccCleanupSite resets the site subsystem and restarts the primary
// server so a test that ends before its own destroy leaves the shared
// server with the site defaults: the SDK skips CheckDestroy when a step
// fails, so this is the safety net. It is registered from PreCheck, so it
// runs only when the test will run.
func testAccCleanupSite(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		admin := testAccClient().S3Admin
		if _, err := admin.DelConfigKV(context.Background(), "site"); err != nil {
			// The test's own destroy already reset the subsystem, in which
			// case the delete reports it as missing.
			if !strings.Contains(strings.ToLower(err.Error()), "not found") &&
				!strings.Contains(strings.ToLower(err.Error()), "there is no target") {
				t.Errorf("resetting the site config after the test: %v", err)
			}
		}
		if err := testAccRestartPrimaryServer(); err != nil {
			t.Errorf("restarting the primary server after the test: %v", err)
		}
	})
}

func TestBuildServerConfigSiteCfgData(t *testing.T) {
	testCases := []struct {
		name     string
		config   S3MinioServerConfigSite
		expected string
	}{
		{
			name:     "all fields set",
			config:   S3MinioServerConfigSite{Name: "dc1", Region: "us-east-1", Comment: "a comment"},
			expected: `name=dc1 region=us-east-1 comment="a comment"`,
		},
		{
			name:     "name only clears region and comment",
			config:   S3MinioServerConfigSite{Name: "dc1"},
			expected: "name=dc1 region= comment=",
		},
		{
			name:     "name with a space is quoted",
			config:   S3MinioServerConfigSite{Name: "dc 1", Region: "us-east-1"},
			expected: `name="dc 1" region=us-east-1 comment=`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildServerConfigSiteCfgData(&tc.config)
			if got != tc.expected {
				t.Errorf("buildServerConfigSiteCfgData() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestServerConfigSiteRunningConfigMatchesState(t *testing.T) {
	config := &S3MinioServerConfigSite{Name: "tf-site", Region: "us-east-1", Comment: "managed by terraform"}

	// The maps below are the responses measured from the test server, not the
	// output of buildServerConfigSiteCfgData.
	t.Run("pre-restart response with default values does not match", func(t *testing.T) {
		if serverConfigSiteRunningConfigMatchesState(config, map[string]string{
			"name":   "",
			"region": "",
		}) {
			t.Errorf("expected no match for the pre-restart response")
		}
	})

	t.Run("post-restart response matches", func(t *testing.T) {
		if !serverConfigSiteRunningConfigMatchesState(config, map[string]string{
			"name":    "tf-site",
			"region":  "us-east-1",
			"comment": "managed by terraform",
		}) {
			t.Errorf("expected the post-restart response to match")
		}
	})

	t.Run("stale region does not match", func(t *testing.T) {
		if serverConfigSiteRunningConfigMatchesState(config, map[string]string{
			"name":    "tf-site",
			"region":  "eu-west-9",
			"comment": "managed by terraform",
		}) {
			t.Errorf("expected no match for a stale region value")
		}
	})

	cleared := &S3MinioServerConfigSite{Name: "tf-site"}
	t.Run("cleared region and comment match the cleared response", func(t *testing.T) {
		if !serverConfigSiteRunningConfigMatchesState(cleared, map[string]string{
			"name":    "tf-site",
			"region":  "",
			"comment": "",
		}) {
			t.Errorf("expected the cleared post-restart response to match")
		}
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
		CheckDestroy: testAccCheckServerStoredTarget("policy_plugin", "", map[string]string{
			"url":          "",
			"auth_token":   "",
			"enable_http2": "off",
		}),
		Steps: []resource.TestStep{
			{
				PreConfig: func() { testAccSeedPolicyPluginComment(t, stubURL) },
				Config: fmt.Sprintf(`
resource "minio_server_config_policy_plugin" "test" {
  url = %[1]q
}
`, stubURL),
				Check: resource.ComposeTestCheckFunc(
					// An empty Optional string is absent from the flat state, so
					// the check is on absence, not on an empty value.
					resource.TestCheckNoResourceAttr(resourceName, "comment"),
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
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", testAccPolicyPluginStubAddress(t))
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
		t.Skipf("no IP for %q in /etc/hosts: the policy plugin tests need the compose test container", hostname)
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

func TestAccMinioServerConfigApi_newKeys(t *testing.T) {
	resourceName := "minio_server_config_api.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_server_config_api" "test" {
  delete_cleanup_interval     = "7m"
  list_quorum                 = "disk"
  object_max_versions         = "500"
  odirect                     = false
  gzip_objects                = true
  replication_max_workers     = "10"
  replication_max_lrg_workers = "4"
  replication_priority        = "fast"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "delete_cleanup_interval", "7m"),
					resource.TestCheckResourceAttr(resourceName, "list_quorum", "disk"),
					resource.TestCheckResourceAttr(resourceName, "object_max_versions", "500"),
					resource.TestCheckResourceAttr(resourceName, "odirect", "false"),
					resource.TestCheckResourceAttr(resourceName, "gzip_objects", "true"),
					resource.TestCheckResourceAttr(resourceName, "replication_max_workers", "10"),
					resource.TestCheckResourceAttr(resourceName, "replication_max_lrg_workers", "4"),
					resource.TestCheckResourceAttr(resourceName, "replication_priority", "fast"),
					testAccCheckServerStoredTarget("api", "", map[string]string{
						"delete_cleanup_interval":     "7m",
						"list_quorum":                 "disk",
						"object_max_versions":         "500",
						"odirect":                     "off",
						"disable_odirect":             "on",
						"gzip_objects":                "on",
						"replication_max_workers":     "10",
						"replication_max_lrg_workers": "4",
						"replication_priority":        "fast",
					}),
				),
			},
			{
				// With disable_odirect=off MinIO keeps O_DIRECT on although
				// odirect=off, so the plan must show odirect drifting to true.
				// odirect leads because MinIO rejects a leading disable_odirect.
				PreConfig: func() { testAccSeedApiConfig(t, "api odirect=off disable_odirect=off") },
				Config: `
resource "minio_server_config_api" "test" {
  delete_cleanup_interval     = "7m"
  list_quorum                 = "disk"
  object_max_versions         = "500"
  odirect                     = false
  gzip_objects                = true
  replication_max_workers     = "10"
  replication_max_lrg_workers = "4"
  replication_priority        = "fast"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: `
resource "minio_server_config_api" "test" {
  delete_cleanup_interval     = "10m"
  list_quorum                 = "optimal"
  object_max_versions         = "250"
  odirect                     = true
  gzip_objects                = false
  replication_max_workers     = "20"
  replication_max_lrg_workers = "7"
  replication_priority        = "slow"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "delete_cleanup_interval", "10m"),
					resource.TestCheckResourceAttr(resourceName, "list_quorum", "optimal"),
					resource.TestCheckResourceAttr(resourceName, "object_max_versions", "250"),
					resource.TestCheckResourceAttr(resourceName, "odirect", "true"),
					resource.TestCheckResourceAttr(resourceName, "gzip_objects", "false"),
					resource.TestCheckResourceAttr(resourceName, "replication_max_workers", "20"),
					resource.TestCheckResourceAttr(resourceName, "replication_max_lrg_workers", "7"),
					resource.TestCheckResourceAttr(resourceName, "replication_priority", "slow"),
					testAccCheckServerStoredTarget("api", "", map[string]string{
						"delete_cleanup_interval":     "10m",
						"list_quorum":                 "optimal",
						"object_max_versions":         "250",
						"odirect":                     "on",
						"disable_odirect":             "off",
						"gzip_objects":                "off",
						"replication_max_workers":     "20",
						"replication_max_lrg_workers": "7",
						"replication_priority":        "slow",
					}),
				),
			},
		},
	})
}

// testAccSeedApiConfig writes api config outside Terraform, and restarts the
// server so the value is live before the next step plans.
func testAccSeedApiConfig(t *testing.T, kv string) {
	t.Helper()
	if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), kv); err != nil {
		t.Fatalf("writing %q outside Terraform: %v", kv, err)
	}
	if err := testAccRestartPrimaryServer(); err != nil {
		t.Fatalf("restarting the server: %v", err)
	}
}

func TestServerConfigAPIODirectEnabled(t *testing.T) {
	cases := []struct {
		cfg  map[string]string
		want bool
	}{
		{map[string]string{"odirect": "on"}, true},
		{map[string]string{"odirect": "off"}, true},
		{map[string]string{"odirect": "off", "disable_odirect": "off"}, true},
		{map[string]string{"odirect": "off", "disable_odirect": "on"}, false},
		{map[string]string{"odirect": "on", "disable_odirect": "on"}, true},
	}
	for _, c := range cases {
		if got := serverConfigAPIODirectEnabled(c.cfg); got != c.want {
			t.Errorf("serverConfigAPIODirectEnabled(%v) = %v, want %v", c.cfg, got, c.want)
		}
	}
}

func TestValidateServerConfigAPIDeleteCleanupInterval(t *testing.T) {
	cases := []struct {
		value     string
		wantError bool
	}{
		{"5m", false},
		{"90s", false},
		{"1h30m", false},
		{"", true},
		{"five", true},
		{"0s", true},
		{"0", true},
		{"-5m", true},
	}
	for _, c := range cases {
		diags := validateServerConfigAPIDeleteCleanupInterval(c.value, cty.Path{})
		if got := diags.HasError(); got != c.wantError {
			t.Errorf("validateServerConfigAPIDeleteCleanupInterval(%q) error = %v, want %v", c.value, got, c.wantError)
		}
	}
}

func TestAccMinioServerConfigApi_unmanagedValuesKept(t *testing.T) {
	resourceName := "minio_server_config_api.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				// Seed a value the configuration does not manage, before the
				// first apply, so the create must not clear it. PreConfig runs
				// after the TF_ACC guard.
				PreConfig: func() { testAccSeedApiConfig(t, "api delete_cleanup_interval=9m") },
				Config: `
resource "minio_server_config_api" "test" {
  stale_uploads_expiry = "12h"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "stale_uploads_expiry", "12h"),
					resource.TestCheckResourceAttr(resourceName, "delete_cleanup_interval", "9m"),
					testAccCheckServerStoredTarget("api", "", map[string]string{
						"stale_uploads_expiry":    "12h",
						"delete_cleanup_interval": "9m",
					}),
				),
			},
			{
				// The old configuration shape must plan no change after the
				// upgrade: the new attributes are unset, so the server keeps
				// their values.
				Config: `
resource "minio_server_config_api" "test" {
  stale_uploads_expiry = "12h"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}
