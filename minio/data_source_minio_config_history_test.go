package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioConfigHistory_basic(t *testing.T) {
	resourceName := "data.minio_config_history.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccConfigHistoryConfig_basic(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "entries.#"),
				),
			},
		},
	})
}

func testAccConfigHistoryConfig_basic() string {
	return `
data "minio_config_history" "test" {}
`
}
