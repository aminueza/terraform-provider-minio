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
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceIAMUserConfig(userName, userSecret),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_iam_user.this", "id", userName),
					resource.TestCheckResourceAttr("data.minio_iam_user.this", "name", userName),
					resource.TestCheckResourceAttr("data.minio_iam_user.this", "status", "enabled"),
				),
			},
		},
	})
}

func TestAccDataSourceMinioIAMUser_disabled(t *testing.T) {
	userName := "tfacc-" + acctest.RandString(8)
	userSecret := "s3cr3t-" + acctest.RandString(16)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceIAMUserDisabledConfig(userName, userSecret),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_iam_user.this", "id", userName),
					resource.TestCheckResourceAttr("data.minio_iam_user.this", "status", "disabled"),
				),
			},
		},
	})
}

func TestAccDataSourceMinioIAMUser_withPolicyAndGroup(t *testing.T) {
	userName := "tfacc-" + acctest.RandString(8)
	userSecret := "s3cr3t-" + acctest.RandString(16)
	groupName := "tfacc-grp-" + acctest.RandString(8)
	policyName := "tfacc-pol-" + acctest.RandString(8)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceIAMUserWithPolicyAndGroupConfig(userName, userSecret, groupName, policyName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.minio_iam_user.this", "id", userName),
					resource.TestCheckResourceAttr("data.minio_iam_user.this", "status", "enabled"),
					resource.TestCheckResourceAttr("data.minio_iam_user.this", "policy_names.#", "1"),
					resource.TestCheckResourceAttr("data.minio_iam_user.this", "policy_names.0", policyName),
					resource.TestCheckResourceAttr("data.minio_iam_user.this", "member_of_groups.#", "1"),
					resource.TestCheckResourceAttr("data.minio_iam_user.this", "member_of_groups.0", groupName),
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

func testAccDataSourceIAMUserDisabledConfig(name, secret string) string {
	return `
provider "minio" {}

resource "minio_iam_user" "test" {
  name         = "` + name + `"
  secret       = "` + secret + `"
  disable_user = true
}

data "minio_iam_user" "this" {
  name = minio_iam_user.test.name
}
`
}

func testAccDataSourceIAMUserWithPolicyAndGroupConfig(name, secret, group, policy string) string {
	return `
provider "minio" {}

resource "minio_iam_user" "test" {
  name   = "` + name + `"
  secret = "` + secret + `"
}

resource "minio_iam_policy" "test" {
  name   = "` + policy + `"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject"]
      Resource = ["arn:aws:s3:::*"]
    }]
  })
}

resource "minio_iam_user_policy_attachment" "test" {
  user_name   = minio_iam_user.test.name
  policy_name = minio_iam_policy.test.name
}

resource "minio_iam_group" "test" {
  name = "` + group + `"
}

resource "minio_iam_group_user_attachment" "test" {
  group_name = minio_iam_group.test.name
  user_name  = minio_iam_user.test.name
}

data "minio_iam_user" "this" {
  name = minio_iam_user.test.name

  depends_on = [
    minio_iam_user_policy_attachment.test,
    minio_iam_group_user_attachment.test,
  ]
}
`
}
