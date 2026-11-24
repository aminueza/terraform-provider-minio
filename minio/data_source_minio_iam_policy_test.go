package minio

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceMinioIAMPolicy_basic(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
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

func TestAccDataSourceMinioIAMPolicy_notFound(t *testing.T) {
	name := "tfacc-missing-" + acctest.RandString(8)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "minio_iam_policy" "test" {
  name = %q
}
`, name),
				ExpectError: regexp.MustCompile(`(?s)\[FATAL\] reading IAM policy.*Server Response Details`),
			},
		},
	})
}
