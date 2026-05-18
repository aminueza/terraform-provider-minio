package minio

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioBatchJobs_basic(t *testing.T) {
	// Batch job tests require multi-cluster replication and source-bucket setup
	// not available in the shared CI MinIO fixture. Enable manually with
	// SKIP_BATCH_JOB_TEST=0.
	if os.Getenv("SKIP_BATCH_JOB_TEST") != "0" {
		t.Skip("Skipping batch job tests: set SKIP_BATCH_JOB_TEST=0 to run")
	}

	resourceName := "data.minio_batch_jobs.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioBatchJobsConfig(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "jobs"),
				),
			},
		},
	})
}

func TestAccDataSourceMinioBatchJobs_filterByType(t *testing.T) {
	// Batch job tests require multi-cluster replication and source-bucket setup
	// not available in the shared CI MinIO fixture. Enable manually with
	// SKIP_BATCH_JOB_TEST=0.
	if os.Getenv("SKIP_BATCH_JOB_TEST") != "0" {
		t.Skip("Skipping batch job tests: set SKIP_BATCH_JOB_TEST=0 to run")
	}

	resourceName := "data.minio_batch_jobs.test"
	jobType := fmt.Sprintf("tfacc-batch-job-type-%d", acctest.RandInt())
	_ = jobType

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioBatchJobsFilterByTypeConfig(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "jobs"),
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
