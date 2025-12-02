package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioIAMUserGroupMembership_basic(t *testing.T) {
	rInt := acctest.RandInt()
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckIAMUserGroupMembershipDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMUserGroupMembershipConfig(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIAMUserGroupMembershipExists("minio_iam_user_group_membership.test"),
					resource.TestCheckResourceAttr("minio_iam_user_group_membership.test", "groups.#", "2"),
				),
			},
		},
	})
}
func testAccCheckIAMUserGroupMembershipExists(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("not found: %s", name)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		conn := testAccProvider.Meta().(*S3MinioClient).S3Admin
		userInfo, err := conn.GetUserInfo(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error getting user info: %w", err)
		}

		expectedGroups := 2
		if len(userInfo.MemberOf) != expectedGroups {
			return fmt.Errorf("expected user to be member of %d groups, got %d", expectedGroups, len(userInfo.MemberOf))
		}

		return nil
	}
}

func testAccCheckIAMUserGroupMembershipDestroy(s *terraform.State) error {
	conn := testAccProvider.Meta().(*S3MinioClient).S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_user_group_membership" {
			continue
		}

		userInfo, err := conn.GetUserInfo(context.Background(), rs.Primary.ID)
		if err != nil {
			// User doesn't exist, which is fine
			continue
		}

		if len(userInfo.MemberOf) > 0 {
			return fmt.Errorf("user %s still has group memberships: %v", rs.Primary.ID, userInfo.MemberOf)
		}
	}

	return nil
}

func testAccIAMUserGroupMembershipConfig(r int) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "user" {
  name = "test-user-%d"
}

resource "minio_iam_group" "group1" {
  name = "group1-%d"
}

resource "minio_iam_group" "group2" {
  name = "group2-%d"
}

resource "minio_iam_user_group_membership" "test" {
  user   = minio_iam_user.user.name
  groups = [minio_iam_group.group1.name, minio_iam_group.group2.name]
}
`, r, r, r)
}
