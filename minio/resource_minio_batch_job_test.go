package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioBatchJob_basic(t *testing.T) {
	jobID := fmt.Sprintf("tfacc-batch-job-%d", acctest.RandInt())
	resourceName := "minio_batch_job.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioBatchJobDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioBatchJobConfig(jobID),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioBatchJobExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "job_type", "expire"),
					resource.TestCheckResourceAttrSet(resourceName, "job_id"),
					resource.TestCheckResourceAttrSet(resourceName, "status"),
				),
			},
		},
	})
}

func testAccCheckMinioBatchJobDestroy(s *terraform.State) error {
	conn := testAccProvider.Meta().(*S3MinioClient).S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_batch_job" {
			continue
		}

		_, err := conn.DescribeBatchJob(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("batch job %s still exists", rs.Primary.ID)
		}
	}

	return nil
}

func testAccCheckMinioBatchJobExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no batch job ID is set")
		}

		conn := testAccProvider.Meta().(*S3MinioClient).S3Admin

		_, err := conn.DescribeBatchJob(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error reading batch job: %w", err)
		}

		return nil
	}
}

func testAccMinioBatchJobConfig(jobID string) string {
	jobYAML := `jobs:
  - name: test-expire-job
    type: expire
    config:
      bucket: test-bucket
      prefix: test-prefix/
      expire-days: 30
`
	return fmt.Sprintf(`
resource "minio_batch_job" "test" {
  job_type = "expire"
  job_yaml = <<-EOF
%[1]s
EOF
}
`, jobYAML)
}
