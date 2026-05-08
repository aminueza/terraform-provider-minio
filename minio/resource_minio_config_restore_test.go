package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioConfigRestore_basic(t *testing.T) {
	resourceName := "minio_config_restore.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioConfigRestoreDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccConfigRestoreConfig_basic(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "restore_id"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckMinioConfigRestoreDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_config_restore" {
			continue
		}
		if rs.Primary.ID != "" {
			return fmt.Errorf("minio_config_restore %s still exists in state after destroy", rs.Primary.ID)
		}
	}
	return nil
}

func testAccConfigRestoreConfig_basic() string {
	return `
data "minio_config_history" "test" {}

resource "minio_config_restore" "test" {
  restore_id = data.minio_config_history.test.entries[0].restore_id
}
`
}
