package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioIAMServiceAccounts_basic(t *testing.T) {
	userName := "tfacc-user-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_iam_user" "test" {
  name = "` + userName + `"
}

data "minio_iam_service_accounts" "test" {
  user = minio_iam_user.test.name
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_iam_service_accounts.test", "service_accounts.#"),
				),
			},
		},
	})
}
