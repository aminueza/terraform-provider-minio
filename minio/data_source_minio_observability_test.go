package minio

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
)

func TestAccDataSourceMinioAccountInfo_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `data "minio_account_info" "test" {}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_account_info.test", "account_name"),
					resource.TestCheckResourceAttrSet("data.minio_account_info.test", "bucket_count"),
					resource.TestCheckResourceAttrSet("data.minio_account_info.test", "total_size"),
					resource.TestCheckResourceAttrSet("data.minio_account_info.test", "total_objects"),
				),
			},
		},
	})
}

func TestAccDataSourceMinioAccountInfo_taggedBucket(t *testing.T) {
	bucketName := "tfacc-acctags-" + acctest.RandString(8)
	bucketConfig := fmt.Sprintf(`
resource "minio_s3_bucket" "tagged" {
  bucket = %q
  tags = {
    team = "storage"
  }
}
`, bucketName)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: bucketConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_s3_bucket.tagged", "tags.team", "storage"),
					testAccWaitForAccountInfoBucket(bucketName),
				),
			},
			{
				Config: bucketConfig + `
data "minio_account_info" "test" {
  depends_on = [minio_s3_bucket.tagged]
}
`,
				Check: resource.TestCheckTypeSetElemNestedAttrs("data.minio_account_info.test", "buckets.*", map[string]string{
					"name": bucketName,
				}),
			},
		},
	})
}

// testAccWaitForAccountInfoBucket waits until accountinfo lists the bucket.
// MinIO builds that list from a bucket cache that is refreshed in the
// background at most once a second and serves the last value meanwhile
// (ListBuckets with Cached in cmd/erasure-server-pool.go), so a bucket created
// a moment ago can be missing from the first answer. Any error other than
// the bucket not being listed yet fails at once, so a decode failure is never
// retried away.
func testAccWaitForAccountInfoBucket(bucket string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		deadline := time.Now().Add(30 * time.Second)
		for {
			info, err := testAccClient().S3Admin.AccountInfo(context.Background(), madmin.AccountOpts{})
			if err != nil {
				return fmt.Errorf("reading account info: %w", err)
			}
			for _, b := range info.Buckets {
				if b.Name == bucket {
					return nil
				}
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("account info did not list bucket %q within 30s", bucket)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func TestAccDataSourceMinioStorageInfo_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `data "minio_storage_info" "test" {}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_storage_info.test", "disk_count"),
					resource.TestCheckResourceAttrSet("data.minio_storage_info.test", "online_disks"),
					resource.TestCheckResourceAttrSet("data.minio_storage_info.test", "total_space"),
					resource.TestCheckResourceAttrSet("data.minio_storage_info.test", "used_space"),
					resource.TestCheckResourceAttrSet("data.minio_storage_info.test", "available_space"),
				),
			},
		},
	})
}

func TestAccDataSourceMinioDataUsage_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `data "minio_data_usage" "test" {}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_data_usage.test", "last_update"),
					resource.TestCheckResourceAttrSet("data.minio_data_usage.test", "total_objects"),
					resource.TestCheckResourceAttrSet("data.minio_data_usage.test", "total_size"),
					resource.TestCheckResourceAttrSet("data.minio_data_usage.test", "buckets_count"),
				),
			},
		},
	})
}

func TestAccDataSourceMinioILMTierStats_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `data "minio_ilm_tier_stats" "test" {}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_ilm_tier_stats.test", "tiers.#"),
				),
			},
		},
	})
}
