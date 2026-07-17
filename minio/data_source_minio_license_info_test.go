package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioLicenseInfo_basic(t *testing.T) {
	dataSourceName := "data.minio_license_info.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: `data "minio_license_info" "test" {}`,
				Check: resource.ComposeTestCheckFunc(
					// The test fixture runs community MinIO, which has no
					// license subsystem, so the read reports the unlicensed
					// fallback instead of failing.
					resource.TestCheckResourceAttr(dataSourceName, "id", "unlicensed"),
					resource.TestCheckResourceAttr(dataSourceName, "plan", ""),
				),
			},
		},
	})
}
