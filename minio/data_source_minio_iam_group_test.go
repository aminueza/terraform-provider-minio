package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioIAMGroup_basic(t *testing.T) {
	groupName := "tfacc-group-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceIAMGroupConfig(groupName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_iam_group.test", "name", groupName),
					resource.TestCheckResourceAttrSet("data.minio_iam_group.test", "status"),
				),
			},
		},
	})
}

func TestAccDataSourceMinioIAMGroups_basic(t *testing.T) {
	groupName := "tfacc-group-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceIAMGroupsConfig(groupName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_iam_groups.all", "groups.#"),
				),
			},
		},
	})
}

func testAccDataSourceIAMGroupConfig(name string) string {
	return `
resource "minio_iam_group" "test" {
  name = "` + name + `"
}

data "minio_iam_group" "test" {
  name = minio_iam_group.test.name
}
`
}

func testAccDataSourceIAMGroupsConfig(name string) string {
	return `
resource "minio_iam_group" "test" {
  name = "` + name + `"
}

data "minio_iam_groups" "all" {
  depends_on = [minio_iam_group.test]
}
`
}
