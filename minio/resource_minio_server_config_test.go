package minio

import (
	"context"
	"fmt"
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
