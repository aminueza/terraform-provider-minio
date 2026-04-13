package minio

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioS3BucketReplication_basic(t *testing.T) {
	bucketName := acctest.RandomWithPrefix("tf-acc-test-a")
	secondBucketName := acctest.RandomWithPrefix("tf-acc-test-b")
	username := acctest.RandomWithPrefix("tf-acc-usr")

	primaryMinioEndpoint := os.Getenv("MINIO_ENDPOINT")
	secondaryMinioEndpoint := os.Getenv("SECOND_MINIO_ENDPOINT")

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccBucketReplicationConfigLocals(primaryMinioEndpoint, secondaryMinioEndpoint) +
					testAccBucketReplicationConfigBucket("my_bucket_in_a", "minio", bucketName) +
					testAccBucketReplicationConfigBucket("my_bucket_in_b", "secondminio", secondBucketName) +
					testAccBucketReplicationConfigPolicy(bucketName, secondBucketName) +
					testAccBucketReplicationConfigServiceAccount(username, 2) +
					kOneWaySimpleResource + `

data "minio_s3_bucket_replication" "test" {
  bucket = minio_s3_bucket_replication.replication_in_b.bucket
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_s3_bucket_replication.test", "bucket", bucketName),
					resource.TestCheckResourceAttr("data.minio_s3_bucket_replication.test", "rule.#", "1"),
					resource.TestCheckResourceAttr("data.minio_s3_bucket_replication.test", "rule.0.enabled", "true"),
					resource.TestCheckResourceAttr("data.minio_s3_bucket_replication.test", "rule.0.delete_replication", "true"),
					resource.TestCheckResourceAttr("data.minio_s3_bucket_replication.test", "rule.0.delete_marker_replication", "true"),
					resource.TestCheckResourceAttr("data.minio_s3_bucket_replication.test", "rule.0.existing_object_replication", "true"),
					resource.TestCheckResourceAttrSet("data.minio_s3_bucket_replication.test", "rule.0.destination.0.bucket"),
					resource.TestCheckResourceAttrSet("data.minio_s3_bucket_replication.test", "rule.0.destination.0.host"),
				),
			},
		},
	})
}
