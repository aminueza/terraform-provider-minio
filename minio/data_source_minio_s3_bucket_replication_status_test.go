package minio

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioS3BucketReplicationStatus_basic(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-ds-replstatus")
	dataSourceName := "data.minio_s3_bucket_replication_status.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceBucketReplicationStatusConfig(bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "bucket", bucketName),
					// A bucket without replication configuration reports zero rules.
					resource.TestCheckResourceAttr(dataSourceName, "rule_count", "0"),
				),
			},
		},
	})
}

func TestAccDataSourceMinioS3BucketReplicationStatus_withReplication(t *testing.T) {
	if os.Getenv("SECOND_MINIO_ENDPOINT") == "" {
		t.Skip("Skipping replication acceptance test: SECOND_MINIO_ENDPOINT is not set")
	}

	bucketName := acctest.RandomWithPrefix("tf-acc-ds-replstatus-a")
	secondBucketName := acctest.RandomWithPrefix("tf-acc-ds-replstatus-b")
	username := acctest.RandomWithPrefix("tf-acc-usr")
	dataSourceName := "data.minio_s3_bucket_replication_status.test"

	primaryMinioEndpoint := os.Getenv("MINIO_ENDPOINT")
	secondaryMinioEndpoint := os.Getenv("SECOND_MINIO_ENDPOINT")

	// Not parallel: remote target endpoints conflict across replication tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketReplicationConfigLocals(primaryMinioEndpoint, secondaryMinioEndpoint) +
					testAccBucketReplicationConfigBucket("my_bucket_in_a", "minio", bucketName) +
					testAccBucketReplicationConfigBucket("my_bucket_in_b", "secondminio", secondBucketName) +
					testAccBucketReplicationConfigPolicy(bucketName, secondBucketName) +
					testAccBucketReplicationConfigServiceAccount(username, 2) +
					kOneWaySimpleResource + `
data "minio_s3_bucket_replication_status" "test" {
  bucket = minio_s3_bucket_replication.replication_in_b.bucket
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "bucket", bucketName),
					resource.TestCheckResourceAttr(dataSourceName, "rule_count", "1"),
					resource.TestCheckResourceAttr(dataSourceName, "rules.#", "1"),
					resource.TestCheckResourceAttr(dataSourceName, "rules.0.status", "Enabled"),
					resource.TestCheckResourceAttrSet(dataSourceName, "rules.0.id"),
					resource.TestCheckResourceAttrSet(dataSourceName, "rules.0.target"),
				),
			},
		},
	})
}

func testAccDataSourceBucketReplicationStatusConfig(bucketName string) string {
	return fmt.Sprintf(`
resource "minio_s3_bucket" "test" {
  bucket = %[1]q
}

data "minio_s3_bucket_replication_status" "test" {
  bucket = minio_s3_bucket.test.bucket
}
`, bucketName)
}
