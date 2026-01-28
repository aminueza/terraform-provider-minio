package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioIAMUser_basic(t *testing.T) {
	userName := "tfacc-" + acctest.RandString(8)
	userSecret := "s3cr3t-" + acctest.RandString(16)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceIAMUserConfig(userName, userSecret),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_user.test", "name", userName),
					resource.TestCheckResourceAttr("data.minio_iam_user.this", "id", userName),
					resource.TestCheckResourceAttr("data.minio_iam_user.this", "status", "enabled"),
				),
			},
		},
	})
}

func testAccDataSourceIAMUserConfig(name, secret string) string {
	return `
provider "minio" {}

resource "minio_iam_user" "test" {
  name   = "` + name + `"
  secret = "` + secret + `"
}

data "minio_iam_user" "this" {
  name = minio_iam_user.test.name
}
`
}