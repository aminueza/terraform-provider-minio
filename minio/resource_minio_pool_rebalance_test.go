package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// TestAccMinioPoolRebalance_basic tests the pool rebalance resource.
// Note: rebalance is only meaningful in multi-pool deployments. In a
// single-pool environment the operation is a no-op but the resource
// lifecycle (create/read/delete) still exercises the API correctly.
func TestAccMinioPoolRebalance_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioPoolRebalanceConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("minio_pool_rebalance.test", "id"),
					resource.TestCheckResourceAttrSet("minio_pool_rebalance.test", "started_at"),
					resource.TestCheckResourceAttrSet("minio_pool_rebalance.test", "status"),
				),
			},
		},
	})
}

const testAccMinioPoolRebalanceConfig = `
resource "minio_pool_rebalance" "test" {}
`
