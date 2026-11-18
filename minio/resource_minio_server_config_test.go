package minio

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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
