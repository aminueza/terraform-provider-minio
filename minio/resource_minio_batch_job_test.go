package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioBatchJob_basic(t *testing.T) {
	t.Skip("Batch job tests require a pre-configured bucket and multi-cluster replication setup not available in the shared CI fixture. To run manually, create a bucket and set up the required replication configuration, then run with TF_ACC=1.")
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
