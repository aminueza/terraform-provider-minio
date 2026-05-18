package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioPoolStatus_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
		},
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioPoolStatusConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_pool_status.test", "pools.#"),
					resource.TestCheckResourceAttrSet("data.minio_pool_status.test", "pools.0.endpoint"),
					resource.TestCheckResourceAttrSet("data.minio_pool_status.test", "pools.0.last_update"),
					resource.TestCheckResourceAttrSet("data.minio_pool_status.test", "pools.0.state"),
				),
			},
		},
	})
}

const testAccDataSourceMinioPoolStatusConfig = `
data "minio_pool_status" "test" {}
`
