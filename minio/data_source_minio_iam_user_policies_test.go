package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioIAMUserPolicies_basic(t *testing.T) {
	userName := "tfacc-audit-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_iam_user" "test" {
  name = "` + userName + `"
}

resource "minio_iam_user_policy_attachment" "test" {
  user_name   = minio_iam_user.test.name
  policy_name = "readonly"
}

data "minio_iam_user_policies" "test" {
  name = minio_iam_user.test.name
  depends_on = [minio_iam_user_policy_attachment.test]
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.minio_iam_user_policies.test", "all_policies.#"),
				),
			},
		},
	})
}
