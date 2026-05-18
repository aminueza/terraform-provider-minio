package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioPoolRebalanceStatus_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioPoolRebalanceStatusConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_pool_rebalance_status.test", "id"),
					resource.TestCheckResourceAttrSet("data.minio_pool_rebalance_status.test", "status"),
				),
			},
		},
	})
}

const testAccDataSourceMinioPoolRebalanceStatusConfig = `
data "minio_pool_rebalance_status" "test" {}
`
