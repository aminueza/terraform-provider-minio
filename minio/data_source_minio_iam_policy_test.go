package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioIAMPolicy_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "minio_iam_policy" "test" {
  name = "readonly"
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_iam_policy.test", "name", "readonly"),
					resource.TestCheckResourceAttrSet("data.minio_iam_policy.test", "policy"),
				),
			},
		},
	})
}
