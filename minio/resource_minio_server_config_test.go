package minio

import (
	"context"
	"os"
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
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				PreConfig: func() { testAccSeedSiteOutsideTerraform(t) },
				Config: `
resource "minio_server_config_site" "test" {
  name = "tf-site"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "tf-site"),
					resource.TestCheckResourceAttr(resourceName, "region", ""),
					resource.TestCheckResourceAttr(resourceName, "comment", ""),
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
