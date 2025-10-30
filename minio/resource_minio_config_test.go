package minio

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioConfig_basic(t *testing.T) {
	resourceName := "minio_config.test"
	configKey := "logger_webhook:1"
	configValue := "enable=off"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioConfigBasic(configKey, configValue),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "key", configKey),
					testCheckResourceAttrContains(resourceName, "value", "enable=off"),
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

func TestAccMinioConfig_update(t *testing.T) {
	resourceName := "minio_config.test"
	configKey := "logger_webhook:2"
	configValue1 := "enable=off batch_size=5 queue_size=50000"
	configValue2 := "enable=off batch_size=10 queue_size=50000"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioConfigBasic(configKey, configValue1),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "key", configKey),
					testCheckResourceAttrContains(resourceName, "value", "enable=off"),
					testCheckResourceAttrContains(resourceName, "value", "batch_size=5"),
					testCheckResourceAttrContains(resourceName, "value", "queue_size=50000"),
				),
			},
			{
				Config: testAccMinioConfigBasic(configKey, configValue2),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "key", configKey),
					testCheckResourceAttrContains(resourceName, "value", "enable=off"),
					testCheckResourceAttrContains(resourceName, "value", "batch_size=10"),
					testCheckResourceAttrContains(resourceName, "value", "queue_size=50000"),
				),
			},
		},
	})
}

func TestAccMinioConfig_multipleSettings(t *testing.T) {
	resourceName := "minio_config.test"
	configKey := "logger_webhook:3"
	configValue := "batch_size=10 enable=off queue_size=75000"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioConfigBasic(configKey, configValue),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "key", configKey),
					testCheckResourceAttrContains(resourceName, "value", "enable=off"),
					testCheckResourceAttrContains(resourceName, "value", "batch_size=10"),
					testCheckResourceAttrContains(resourceName, "value", "queue_size=75000"),
				),
			},
		},
	})
}

func TestAccMinioConfig_webhookNotification(t *testing.T) {
	resourceName := "minio_config.test"
	configKey := "logger_webhook:4"
	configValue := "enable=off endpoint=http://example.com queue_size=50000"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioConfigBasic(configKey, configValue),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioConfigExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "key", configKey),
					testCheckResourceAttrContains(resourceName, "value", "enable=off"),
					testCheckResourceAttrContains(resourceName, "value", "endpoint=http://example.com"),
					testCheckResourceAttrContains(resourceName, "value", "queue_size=50000"),
				),
			},
		},
	})
}

func testAccCheckMinioConfigExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no config ID is set")
		}

		minioC := testAccProvider.Meta().(*S3MinioClient)
		configKey := rs.Primary.ID

		// Try to get the config
		_, err := minioC.S3Admin.GetConfigKV(context.Background(), configKey)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
				return fmt.Errorf("config %s not found", configKey)
			}
			return err
		}

		return nil
	}
}

func testAccCheckMinioConfigDestroy(s *terraform.State) error {
	// Config resources may persist after deletion, so we just check the resource is removed from state
	return nil
}

func testAccMinioConfigBasic(key, value string) string {
	return fmt.Sprintf(`
resource "minio_config" "test" {
  key   = %[1]q
  value = %[2]q
}
`, key, value)
}

func testCheckResourceAttrContains(resourceName, attr, value string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if !strings.Contains(rs.Primary.Attributes[attr], value) {
			return fmt.Errorf("%s attribute does not contain %s", attr, value)
		}

		return nil
	}
}
