package minio

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccMinioPoolDecommission_basic(t *testing.T) {
	if os.Getenv("RUN_POOL_DECOMMISSION_ACC") != "1" {
		t.Skip("skipping pool decommission test; set RUN_POOL_DECOMMISSION_ACC=1 to enable")
	}

	resourceName := "minio_pool_decommission.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
		},
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioPoolDecommissionConfig(0),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "started_at"),
					resource.TestCheckResourceAttrSet(resourceName, "status"),
				),
			},
		},
	})
}

func testAccMinioPoolDecommissionConfig(poolIndex int) string {
	return fmt.Sprintf(`
resource "minio_pool_decommission" "test" {
  pool_index = %d
}
`, poolIndex)
}
