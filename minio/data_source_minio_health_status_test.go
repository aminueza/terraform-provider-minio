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
					// Verify ID is set (timestamp)
					resource.TestCheckResourceAttrSet("data.minio_health_status.test", "id"),
					// For a functional MinIO test instance, all health checks should pass
					resource.TestCheckResourceAttr("data.minio_health_status.test", "live", "true"),
					resource.TestCheckResourceAttr("data.minio_health_status.test", "ready", "true"),
					resource.TestCheckResourceAttr("data.minio_health_status.test", "write_quorum", "true"),
					resource.TestCheckResourceAttr("data.minio_health_status.test", "read_quorum", "true"),
					resource.TestCheckResourceAttr("data.minio_health_status.test", "healthy", "true"),
					// safe_for_maintenance should be true for a healthy standalone instance
					resource.TestCheckResourceAttr("data.minio_health_status.test", "safe_for_maintenance", "true"),
					// Verify outputs contain the actual data
					resource.TestCheckOutput("health_live", "true"),
					resource.TestCheckOutput("health_ready", "true"),
					resource.TestCheckOutput("health_overall", "true"),
				),
			},
		},
	})
}

func TestAccDataSourceMinioHealthStatus_multipleReads(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioHealthStatusConfigMultiple(),
				Check: resource.ComposeTestCheckFunc(
					// First data source
					resource.TestCheckResourceAttrSet("data.minio_health_status.first", "id"),
					resource.TestCheckResourceAttrSet("data.minio_health_status.first", "healthy"),
					// Second data source
					resource.TestCheckResourceAttrSet("data.minio_health_status.second", "id"),
					resource.TestCheckResourceAttrSet("data.minio_health_status.second", "healthy"),
					// Both should report same health status
					resource.TestCheckResourceAttrPair(
						"data.minio_health_status.first", "live",
						"data.minio_health_status.second", "live",
					),
					resource.TestCheckResourceAttrPair(
						"data.minio_health_status.first", "ready",
						"data.minio_health_status.second", "ready",
					),
				),
			},
		},
	})
}

func TestAccDataSourceMinioHealthStatus_withOutputs(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioHealthStatusConfigWithOutputs(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_health_status.test", "id"),
					resource.TestCheckResourceAttrSet("data.minio_health_status.test", "healthy"),
				),
			},
		},
	})
}

func testAccDataSourceMinioHealthStatusConfig() string {
	return `
provider "minio" {}

data "minio_health_status" "test" {}

output "health_live" {
  value = data.minio_health_status.test.live
}

output "health_ready" {
  value = data.minio_health_status.test.ready
}

output "health_overall" {
  value = data.minio_health_status.test.healthy
}
`
}

func testAccDataSourceMinioHealthStatusConfigMultiple() string {
	return `
provider "minio" {}

data "minio_health_status" "first" {}

data "minio_health_status" "second" {}
`
}

func testAccDataSourceMinioHealthStatusConfigWithOutputs() string {
	return `
provider "minio" {}

data "minio_health_status" "test" {}

output "cluster_healthy" {
  value = data.minio_health_status.test.healthy
}

output "cluster_live" {
  value = data.minio_health_status.test.live
}

output "cluster_ready" {
  value = data.minio_health_status.test.ready
}

output "write_quorum_ok" {
  value = data.minio_health_status.test.write_quorum
}

output "read_quorum_ok" {
  value = data.minio_health_status.test.read_quorum
}

output "safe_for_maintenance" {
  value = data.minio_health_status.test.safe_for_maintenance
}
`
}
