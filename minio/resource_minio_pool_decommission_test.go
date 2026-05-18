package minio

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccMinioPoolDecommission_basic(t *testing.T) {
	if os.Getenv("SKIP_DECOMMISSION_TEST") != "0" {
		t.Skip("skipping decommission test (requires multi-pool MinIO); set SKIP_DECOMMISSION_TEST=0 to enable")
	}

	resourceName := "minio_pool_decommission.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			t.Log("SKIP: pool decommission requires a multi-pool MinIO cluster; skipping in single-pool deployments")
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
	r := acctest.RandString(8)

	return fmt.Sprintf(`
resource "minio_pool_decommission" "test" {
  pool_index = %d
}
`, poolIndex) + fmt.Sprintf(`
# Random suffix to avoid conflicts: %s
`, r)
}
