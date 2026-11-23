package minio

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

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

data "minio_iam_users" "enabled" {
  depends_on  = [minio_iam_user.u1, minio_iam_user.u2]
  name_prefix = "` + prefix + `-"
  status      = "enabled"
}
`
}

func TestAccDataSourceMinioIAMUsers_policiesGroupsSortedAndStatus(t *testing.T) {
	prefix := "tfacc-usrs-" + acctest.RandString(5) + "-"
	aUser := prefix + "a-user"
	dUser := prefix + "d-user"
	mUser := prefix + "m-user"
	zUser := prefix + "z-user"
	secret := "s3cr3t-" + acctest.RandString(16)
	policy1 := "tfacc-pol1-" + acctest.RandString(6)
	policy2 := "tfacc-pol2-" + acctest.RandString(6)
	groupName := "tfacc-grp-" + acctest.RandString(6)

	cfg := fmt.Sprintf(`
provider "minio" {}

resource "minio_iam_user" "z_user" {
  name   = %[2]q
  secret = %[5]q
}

resource "minio_iam_user" "d_user" {
  name         = %[3]q
  secret       = %[5]q
  disable_user = true
}

resource "minio_iam_user" "a_user" {
  name   = %[1]q
  secret = %[5]q
}

resource "minio_iam_user" "m_user" {
  name   = %[4]q
  secret = %[5]q
}

resource "minio_iam_policy" "p1" {
  name = %[6]q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject"]
      Resource = ["arn:aws:s3:::*"]
    }]
  })
}

resource "minio_iam_policy" "p2" {
  name = %[7]q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:ListBucket"]
      Resource = ["arn:aws:s3:::*"]
    }]
  })
}

resource "minio_iam_user_policy_attachment" "a_p1" {
  user_name   = minio_iam_user.a_user.name
  policy_name = minio_iam_policy.p1.name
}

resource "minio_iam_user_policy_attachment" "a_p2" {
  user_name   = minio_iam_user.a_user.name
  policy_name = minio_iam_policy.p2.name
}

resource "minio_iam_group" "g1" {
  name = %[8]q
}

resource "minio_iam_group_user_attachment" "a_g1" {
  group_name = minio_iam_group.g1.name
  user_name  = minio_iam_user.a_user.name
}

data "minio_iam_user" "a" {
  name = minio_iam_user.a_user.name

  depends_on = [
    minio_iam_user_policy_attachment.a_p1,
    minio_iam_user_policy_attachment.a_p2,
    minio_iam_group_user_attachment.a_g1,
  ]
}

data "minio_iam_users" "enabled" {
  name_prefix = %[9]q
  status      = "enabled"

  depends_on = [
    minio_iam_user.z_user,
    minio_iam_user.d_user,
    minio_iam_user.a_user,
    minio_iam_user.m_user,
    minio_iam_user_policy_attachment.a_p1,
    minio_iam_user_policy_attachment.a_p2,
    minio_iam_group_user_attachment.a_g1,
  ]
}

data "minio_iam_users" "disabled" {
  name_prefix = %[9]q
  status      = "disabled"

  depends_on = [
    minio_iam_user.z_user,
    minio_iam_user.d_user,
    minio_iam_user.a_user,
    minio_iam_user.m_user,
  ]
}

data "minio_iam_users" "all" {
  name_prefix = %[9]q
  status      = "all"

  depends_on = [
    minio_iam_user.z_user,
    minio_iam_user.d_user,
    minio_iam_user.a_user,
    minio_iam_user.m_user,
  ]
}
`, aUser, zUser, dUser, mUser, secret, policy1, policy2, groupName, prefix)

	var firstEnabledID string
	checkStep := resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttr("data.minio_iam_users.enabled", "users.#", "3"),
		resource.TestCheckResourceAttr("data.minio_iam_users.enabled", "users.0.name", aUser),
		resource.TestCheckResourceAttr("data.minio_iam_users.enabled", "users.1.name", mUser),
		resource.TestCheckResourceAttr("data.minio_iam_users.enabled", "users.2.name", zUser),
		resource.TestCheckResourceAttr("data.minio_iam_users.enabled", "users.0.policy_names.#", "2"),
		resource.TestCheckTypeSetElemAttr("data.minio_iam_users.enabled", "users.0.policy_names.*", policy1),
		resource.TestCheckTypeSetElemAttr("data.minio_iam_users.enabled", "users.0.policy_names.*", policy2),
		resource.TestCheckResourceAttr("data.minio_iam_users.enabled", "users.0.member_of_groups.#", "1"),
		resource.TestCheckTypeSetElemAttr("data.minio_iam_users.enabled", "users.0.member_of_groups.*", groupName),
		resource.TestCheckResourceAttrPair("data.minio_iam_users.enabled", "users.0.policy_names.#", "data.minio_iam_user.a", "policy_names.#"),
		resource.TestCheckResourceAttrPair("data.minio_iam_users.enabled", "users.0.member_of_groups.#", "data.minio_iam_user.a", "member_of_groups.#"),
		resource.TestCheckResourceAttr("data.minio_iam_users.enabled", "users.1.policy_names.#", "0"),
		resource.TestCheckResourceAttr("data.minio_iam_users.enabled", "users.1.member_of_groups.#", "0"),
		resource.TestCheckResourceAttr("data.minio_iam_users.disabled", "users.#", "1"),
		resource.TestCheckResourceAttr("data.minio_iam_users.disabled", "users.0.name", dUser),
		resource.TestCheckResourceAttr("data.minio_iam_users.all", "users.#", "4"),
	)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					checkStep,
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["data.minio_iam_users.enabled"]
						if !ok || rs.Primary == nil || rs.Primary.ID == "" {
							return fmt.Errorf("data.minio_iam_users.enabled id not set")
						}
						firstEnabledID = rs.Primary.ID
						return nil
					},
				),
			},
			{
				PreConfig: func() {
					time.Sleep(1100 * time.Millisecond)
				},
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					checkStep,
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["data.minio_iam_users.enabled"]
						if !ok || rs.Primary == nil {
							return fmt.Errorf("data.minio_iam_users.enabled missing in state")
						}
						if rs.Primary.ID != firstEnabledID {
							return fmt.Errorf("expected deterministic id %q across reads, got %q", firstEnabledID, rs.Primary.ID)
						}
						return nil
					},
				),
			},
		},
	})
}
