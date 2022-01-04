package minio

import (
	"context"
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/minio/pkg/madmin"
)

func TestAccMinioGroupMembership_basic(t *testing.T) {
	var group madmin.GroupDesc

	rString := acctest.RandString(8)
	groupName := fmt.Sprintf("tf-acc-group-gm-basic-%s", rString)
	userName := fmt.Sprintf("tf-acc-user-gm-basic-%s", rString)
	userName2 := fmt.Sprintf("tf-acc-user-gm-basic-two-%s", rString)
	userName3 := fmt.Sprintf("tf-acc-user-gm-basic-three-%s", rString)
	membershipName := fmt.Sprintf("tf-acc-membership-gm-basic-%s", rString)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioGroupMembershipDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioGroupMemberConfig(groupName, userName, membershipName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupMembershipExists("minio_iam_group_membership.team", &group),
					testAccCheckMinioGroupMembershipAttributes(&group, groupName, []string{userName}),
				),
			},
			{
				Config: testAccMinioGroupMemberConfigUpdate(groupName, userName2, userName3, membershipName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupMembershipExists("minio_iam_group_membership.team", &group),
					testAccCheckMinioGroupMembershipAttributes(&group, groupName, []string{userName2, userName3}),
				),
			},
			{
				Config: testAccMinioGroupMemberConfigUpdateDown(groupName, userName3, membershipName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupMembershipExists("minio_iam_group_membership.team", &group),
					testAccCheckMinioGroupMembershipAttributes(&group, groupName, []string{userName3}),
				),
			},
		},
	})
}

func TestAccMinioGroupMembership_paginatedUserList(t *testing.T) {
	var group madmin.GroupDesc

	rString := acctest.RandString(8)
	groupName := fmt.Sprintf("tf-acc-group-gm-pul-%s", rString)
	membershipName := fmt.Sprintf("tf-acc-membership-gm-pul-%s", rString)
	userNamePrefix := fmt.Sprintf("tf-acc-user-gm-pul-%s-", rString)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioGroupMembershipDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioGroupMemberConfigPaginatedUserList(groupName, membershipName, userNamePrefix),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupMembershipExists("minio_iam_group_membership.team", &group),
					resource.TestCheckResourceAttr(
						"minio_iam_group_membership.team", "users.#", "101"),
				),
			},
		},
	})
}

func testAccCheckMinioGroupMembershipDestroy(s *terraform.State) error {
	iamconn := testAccProvider.Meta().(*S3MinioClient).S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_group_membership" {
			continue
		}

		group := rs.Primary.Attributes["group"]

		_, err := iamconn.GetGroupDescription(context.Background(), group)
		if err == nil {
			return fmt.Errorf("Group still exists")
		}
	}

	return nil
}

func testAccCheckMinioGroupMembershipExists(n string, g *madmin.GroupDesc) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No User name is set")
		}

		iamconn := testAccProvider.Meta().(*S3MinioClient).S3Admin
		gn := rs.Primary.Attributes["group"]

		resp, err := iamconn.GetGroupDescription(context.Background(), gn)
		if err != nil {
			return fmt.Errorf("Error: Group (%s) not found", gn)
		}

		log.Printf("MEMBERS: %v", resp.Members)

		*g = *resp

		return nil
	}
}

func testAccCheckMinioGroupMembershipAttributes(group *madmin.GroupDesc, groupName string, users []string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if !strings.Contains(group.Name, groupName) {
			return fmt.Errorf("Bad group membership: expected %s, got %s", groupName, group.Name)
		}

		log.Printf("MEMBERS: %v  and %v", group.Members, users)

		uc := len(users)
		for _, u := range users {
			for _, gu := range group.Members {
				if u == gu {
					uc--
				}
			}
		}

		if uc > 0 {
			return fmt.Errorf("Bad group membership count, expected (%v), but only (%v) found", users, group.Members)
		}
		return nil
	}
}

func testAccMinioGroupMemberConfig(groupName, userName, membershipName string) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "group" {
  name = "%s"
}
resource "minio_iam_user" "user" {
  name = "%s"
}
resource "minio_iam_group_membership" "team" {
  name  = "%s"
  users = ["${minio_iam_user.user.name}"]
  group = "${minio_iam_group.group.name}"
}
`, groupName, userName, membershipName)
}

func testAccMinioGroupMemberConfigUpdate(groupName, userName2, userName3, membershipName string) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "group" {
  name = "%s"
}

resource "minio_iam_user" "user_two" {
  name = "%s"
}
resource "minio_iam_user" "user_three" {
  name = "%s"
}
resource "minio_iam_group_membership" "team" {
  name = "%s"
  users = [
    "${minio_iam_user.user_two.name}",
    "${minio_iam_user.user_three.name}",
  ]
  group = "${minio_iam_group.group.name}"
}
`, groupName, userName2, userName3, membershipName)
}

func testAccMinioGroupMemberConfigUpdateDown(groupName, userName3, membershipName string) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "group" {
  name = "%s"
}
resource "minio_iam_user" "user_three" {
  name = "%s"
}
resource "minio_iam_group_membership" "team" {
  name = "%s"
  users = [
    "${minio_iam_user.user_three.name}",
  ]
  group = "${minio_iam_group.group.name}"
}
`, groupName, userName3, membershipName)
}

func testAccMinioGroupMemberConfigPaginatedUserList(groupName, membershipName, userNamePrefix string) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "group" {
  name = "%s"
}
resource "minio_iam_group_membership" "team" {
  name  = "%s"
  group = "${minio_iam_group.group.name}"
  # TODO: Switch back to simple list reference when test configurations are upgraded to 0.12 syntax
  users = [
    "${minio_iam_user.user.*.name[0]}",
    "${minio_iam_user.user.*.name[1]}",
    "${minio_iam_user.user.*.name[2]}",
    "${minio_iam_user.user.*.name[3]}",
    "${minio_iam_user.user.*.name[4]}",
    "${minio_iam_user.user.*.name[5]}",
    "${minio_iam_user.user.*.name[6]}",
    "${minio_iam_user.user.*.name[7]}",
    "${minio_iam_user.user.*.name[8]}",
    "${minio_iam_user.user.*.name[9]}",
    "${minio_iam_user.user.*.name[10]}",
    "${minio_iam_user.user.*.name[11]}",
    "${minio_iam_user.user.*.name[12]}",
    "${minio_iam_user.user.*.name[13]}",
    "${minio_iam_user.user.*.name[14]}",
    "${minio_iam_user.user.*.name[15]}",
    "${minio_iam_user.user.*.name[16]}",
    "${minio_iam_user.user.*.name[17]}",
    "${minio_iam_user.user.*.name[18]}",
    "${minio_iam_user.user.*.name[19]}",
    "${minio_iam_user.user.*.name[20]}",
    "${minio_iam_user.user.*.name[21]}",
    "${minio_iam_user.user.*.name[22]}",
    "${minio_iam_user.user.*.name[23]}",
    "${minio_iam_user.user.*.name[24]}",
    "${minio_iam_user.user.*.name[25]}",
    "${minio_iam_user.user.*.name[26]}",
    "${minio_iam_user.user.*.name[27]}",
    "${minio_iam_user.user.*.name[28]}",
    "${minio_iam_user.user.*.name[29]}",
    "${minio_iam_user.user.*.name[30]}",
    "${minio_iam_user.user.*.name[31]}",
    "${minio_iam_user.user.*.name[32]}",
    "${minio_iam_user.user.*.name[33]}",
    "${minio_iam_user.user.*.name[34]}",
    "${minio_iam_user.user.*.name[35]}",
    "${minio_iam_user.user.*.name[36]}",
    "${minio_iam_user.user.*.name[37]}",
    "${minio_iam_user.user.*.name[38]}",
    "${minio_iam_user.user.*.name[39]}",
    "${minio_iam_user.user.*.name[40]}",
    "${minio_iam_user.user.*.name[41]}",
    "${minio_iam_user.user.*.name[42]}",
    "${minio_iam_user.user.*.name[43]}",
    "${minio_iam_user.user.*.name[44]}",
    "${minio_iam_user.user.*.name[45]}",
    "${minio_iam_user.user.*.name[46]}",
    "${minio_iam_user.user.*.name[47]}",
    "${minio_iam_user.user.*.name[48]}",
    "${minio_iam_user.user.*.name[49]}",
    "${minio_iam_user.user.*.name[50]}",
    "${minio_iam_user.user.*.name[51]}",
    "${minio_iam_user.user.*.name[52]}",
    "${minio_iam_user.user.*.name[53]}",
    "${minio_iam_user.user.*.name[54]}",
    "${minio_iam_user.user.*.name[55]}",
    "${minio_iam_user.user.*.name[56]}",
    "${minio_iam_user.user.*.name[57]}",
    "${minio_iam_user.user.*.name[58]}",
    "${minio_iam_user.user.*.name[59]}",
    "${minio_iam_user.user.*.name[60]}",
    "${minio_iam_user.user.*.name[61]}",
    "${minio_iam_user.user.*.name[62]}",
    "${minio_iam_user.user.*.name[63]}",
    "${minio_iam_user.user.*.name[64]}",
    "${minio_iam_user.user.*.name[65]}",
    "${minio_iam_user.user.*.name[66]}",
    "${minio_iam_user.user.*.name[67]}",
    "${minio_iam_user.user.*.name[68]}",
    "${minio_iam_user.user.*.name[69]}",
    "${minio_iam_user.user.*.name[70]}",
    "${minio_iam_user.user.*.name[71]}",
    "${minio_iam_user.user.*.name[72]}",
    "${minio_iam_user.user.*.name[73]}",
    "${minio_iam_user.user.*.name[74]}",
    "${minio_iam_user.user.*.name[75]}",
    "${minio_iam_user.user.*.name[76]}",
    "${minio_iam_user.user.*.name[77]}",
    "${minio_iam_user.user.*.name[78]}",
    "${minio_iam_user.user.*.name[79]}",
    "${minio_iam_user.user.*.name[80]}",
    "${minio_iam_user.user.*.name[81]}",
    "${minio_iam_user.user.*.name[82]}",
    "${minio_iam_user.user.*.name[83]}",
    "${minio_iam_user.user.*.name[84]}",
    "${minio_iam_user.user.*.name[85]}",
    "${minio_iam_user.user.*.name[86]}",
    "${minio_iam_user.user.*.name[87]}",
    "${minio_iam_user.user.*.name[88]}",
    "${minio_iam_user.user.*.name[89]}",
    "${minio_iam_user.user.*.name[90]}",
    "${minio_iam_user.user.*.name[91]}",
    "${minio_iam_user.user.*.name[92]}",
    "${minio_iam_user.user.*.name[93]}",
    "${minio_iam_user.user.*.name[94]}",
    "${minio_iam_user.user.*.name[95]}",
    "${minio_iam_user.user.*.name[96]}",
    "${minio_iam_user.user.*.name[97]}",
    "${minio_iam_user.user.*.name[98]}",
    "${minio_iam_user.user.*.name[99]}",
    "${minio_iam_user.user.*.name[100]}",
  ]
}
resource "minio_iam_user" "user" {
  count = 101
  name  = "${format("%s%%d", count.index + 1)}"
}
`, groupName, membershipName, userNamePrefix)
}
