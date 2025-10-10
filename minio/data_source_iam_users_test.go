package minio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// Creates two users with a common prefix, then lists them via the data source
// using prefix + status filters. We also assert count=2 for the "enabled" set.
func TestAccDataSourceMinioIAMUsers_listAndFilter(t *testing.T) {
	prefix := "tfacc-" + acctest.RandString(5)
	userName1 := prefix + "-" + acctest.RandString(6)
	userName2 := prefix + "-" + acctest.RandString(6)
	secret1 := "s3cr3t-" + acctest.RandString(16)
	secret2 := "s3cr3t-" + acctest.RandString(16)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceIAMUsersConfig(prefix, userName1, secret1, userName2, secret2),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_user.u1", "name", userName1),
					resource.TestCheckResourceAttr("minio_iam_user.u2", "name", userName2),
					resource.TestCheckResourceAttrSet("data.minio_iam_users.enabled", "id"),
					resource.TestCheckResourceAttr("data.minio_iam_users.enabled", "users.#", "2"),
				),
			},
		},
	})
}

func testAccDataSourceIAMUsersConfig(prefix, name1, secret1, name2, secret2 string) string {
	return `
provider "minio" {}

resource "minio_iam_user" "u1" {
  name   = "` + name1 + `"
  secret = "` + secret1 + `"
}

resource "minio_iam_user" "u2" {
  name   = "` + name2 + `"
  secret = "` + secret2 + `"
}

# Enabled subset (default status is "enabled")
data "minio_iam_users" "enabled" {
  depends_on  = [minio_iam_user.u1, minio_iam_user.u2]
  name_prefix = "` + prefix + `-"
  status      = "enabled"
}
`
}