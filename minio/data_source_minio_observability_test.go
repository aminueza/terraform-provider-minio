package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioAccountInfo_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
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

func TestAccDataSourceMinioStorageInfo_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
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
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
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
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
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
