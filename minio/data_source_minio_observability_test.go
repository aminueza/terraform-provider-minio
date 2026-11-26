package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
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

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioS3BucketDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "minio_s3_bucket" "tagged" {
  bucket = %q
  tags = {
    team = "storage"
  }
}

data "minio_account_info" "test" {
  depends_on = [minio_s3_bucket.tagged]
}
`, bucketName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_s3_bucket.tagged", "tags.team", "storage"),
					resource.TestCheckTypeSetElemNestedAttrs("data.minio_account_info.test", "buckets.*", map[string]string{
						"name": bucketName,
					}),
				),
			},
		},
	})
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
