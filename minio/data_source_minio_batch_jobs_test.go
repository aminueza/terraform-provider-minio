package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioBatchJobs_basic(t *testing.T) {
	resourceName := "data.minio_batch_jobs.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioBatchJobsConfig(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "jobs.#"),
				),
			},
		},
	})
}

func TestAccDataSourceMinioBatchJobs_filterByType(t *testing.T) {
	resourceName := "data.minio_batch_jobs.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioBatchJobsFilterByTypeConfig(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "jobs.#"),
				),
			},
		},
	})
}

func testAccDataSourceMinioBatchJobsConfig() string {
	return `
data "minio_batch_jobs" "test" {
}
`
}

func testAccDataSourceMinioBatchJobsFilterByTypeConfig() string {
	return `
data "minio_batch_jobs" "test" {
  job_type = "replicate"
}
`
}
