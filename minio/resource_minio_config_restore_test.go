package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioConfigRestore_basic(t *testing.T) {
	restoreID := acctest.RandString(8)
	resourceName := "minio_config_restore.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioConfigRestoreDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccConfigRestoreConfig_basic(restoreID),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "restore_id", restoreID),
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
			return nil
		}
	}

	return nil
}

func testAccConfigRestoreConfig_basic(restoreID string) string {
	return `
resource "minio_config_restore" "test" {
  restore_id = "` + restoreID + `"
}
`
}