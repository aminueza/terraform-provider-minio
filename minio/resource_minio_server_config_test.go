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

// testAccScannerStoredConfig restarts the primary test server and checks
// that get-config-kv reports the scanner configuration with the given
// key=value pairs. The values in want are explicit literals from the test's
// own configuration: they assert what the server persisted, not what the
// provider would have written.
func testAccScannerStoredConfig(want map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if err := testAccRestartPrimaryServer(); err != nil {
			return err
		}

		data, err := testAccClient().S3Admin.GetConfigKV(context.Background(), "scanner")
		if err != nil {
			return fmt.Errorf("reading scanner from the server after the restart: %w", err)
		}
		stored := parseConfigParams(string(data))
		for key, value := range want {
			got, ok := stored[key]
			if !ok {
				return fmt.Errorf("scanner stored on the server has no %q key; response: %s", key, data)
			}
			if got != value {
				return fmt.Errorf("%q of scanner stored on the server is %q, want %q", key, got, value)
			}
		}
		return nil
	}
}

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
					testAccScannerStoredConfig(map[string]string{
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
					testAccScannerStoredConfig(map[string]string{
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

// The upgrade path when the configuration does not set cycle: cycle is
// Optional and Computed, so the glued value stored by an older provider
// never shows a normal diff. The CustomizeDiff must force the cycle diff so
// the next apply writes a clean cycle instead of gluing the old keys on
// again. The thresholds equal the server defaults, so the forced cycle diff
// is the only one the apply acts on.
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
				Config: config,
				Check:  resource.TestCheckResourceAttrSet(resourceName, "cycle"),
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
					testAccScannerStoredConfig(map[string]string{
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
