package minio

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
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

// testAccCheckApiConfigStored restarts the primary test server and checks that
// the stored api subsystem configuration carries the given key=value pairs.
// The values are explicit literals from the test's own configuration: they
// assert what the server persisted, not what the provider would have written.
// The api subsystem is a singleton addressed by the bare key "api", so
// testAccCheckServerStoredTarget's "subsystem:name" keying cannot reach it and
// this check pairs testAccRestartPrimaryServer with the bare key instead.
func testAccCheckApiConfigStored(want map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if err := testAccRestartPrimaryServer(); err != nil {
			return err
		}

		client := testAccClient()
		data, err := client.S3Admin.GetConfigKV(context.Background(), "api")
		if err != nil {
			return fmt.Errorf("reading api from the server after the restart: %w", err)
		}
		stored, found := notifyTargetConfigParams(string(data), "api")
		if !found {
			return fmt.Errorf("api is not stored on the server after the restart; response: %s", string(data))
		}
		for key, value := range want {
			got, ok := stored[key]
			if !ok {
				return fmt.Errorf("api stored on the server has no %q key; response: %s", key, string(data))
			}
			if got != value {
				return fmt.Errorf("%q of api stored on the server is %q, want %q", key, got, value)
			}
		}
		return nil
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
					testAccCheckApiConfigStored(map[string]string{
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
					testAccCheckApiConfigStored(map[string]string{
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
					testAccCheckApiConfigStored(map[string]string{
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
