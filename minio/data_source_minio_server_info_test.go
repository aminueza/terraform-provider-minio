package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioServerInfo_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioServerInfoConfig(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_server_info.test", "id"),
					resource.TestCheckResourceAttrSet("data.minio_server_info.test", "version"),
					resource.TestCheckResourceAttrSet("data.minio_server_info.test", "commit"),
					resource.TestCheckResourceAttrSet("data.minio_server_info.test", "region"),
					resource.TestCheckResourceAttrSet("data.minio_server_info.test", "deployment_id"),
					resource.TestCheckResourceAttrSet("data.minio_server_info.test", "servers.#"),
				),
			},
		},
	})
}

func testAccDataSourceMinioServerInfoConfig() string {
	return `
provider "minio" {}

data "minio_server_info" "test" {}
`
}
