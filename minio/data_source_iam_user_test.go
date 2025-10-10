package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// Creates an IAM user, then reads it via the data source.
func TestAccDataSourceMinioIAMUser_basic(t *testing.T) {
	userName := "tfacc-" + acctest.RandString(8)
	userSecret := "s3cr3t-" + acctest.RandString(16)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) }, // from provider_test.go
		ProviderFactories: testAccProviders,              // defined in provider_test.go
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