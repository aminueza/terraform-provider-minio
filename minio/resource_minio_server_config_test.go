package minio

import (
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
