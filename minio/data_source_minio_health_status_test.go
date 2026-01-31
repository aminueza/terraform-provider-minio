package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioHealthStatus_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioHealthStatusConfig(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_health_status.test", "id"),
					resource.TestCheckResourceAttr("data.minio_health_status.test", "live", "true"),
					resource.TestCheckResourceAttr("data.minio_health_status.test", "ready", "true"),
					resource.TestCheckResourceAttrSet("data.minio_health_status.test", "healthy"),
					resource.TestCheckResourceAttrSet("data.minio_health_status.test", "write_quorum"),
					resource.TestCheckResourceAttrSet("data.minio_health_status.test", "read_quorum"),
					resource.TestCheckResourceAttrSet("data.minio_health_status.test", "safe_for_maintenance"),
				),
			},
		},
	})
}

func testAccDataSourceMinioHealthStatusConfig() string {
	return `
provider "minio" {}

data "minio_health_status" "test" {}
`
}
