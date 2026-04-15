package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioS3BucketReplicationMetrics_basic(t *testing.T) {
	bucketName := "tfacc-ds-repmetrics-" + acctest.RandString(8)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceMinioS3BucketReplicationMetricsConfig(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_s3_bucket_replication_metrics.test", "bucket", bucketName),
					resource.TestCheckResourceAttrSet("data.minio_s3_bucket_replication_metrics.test", "pending_size"),
					resource.TestCheckResourceAttrSet("data.minio_s3_bucket_replication_metrics.test", "failed_count"),
					resource.TestCheckResourceAttrSet("data.minio_s3_bucket_replication_metrics.test", "replicated_size"),
				),
			},
		},
	})
}

func testAccDataSourceMinioS3BucketReplicationMetricsConfig(bucketName string) string {
	return `
resource "minio_s3_bucket" "test" {
  bucket = "` + bucketName + `"
}

data "minio_s3_bucket_replication_metrics" "test" {
  bucket = minio_s3_bucket.test.bucket
}
`
}
