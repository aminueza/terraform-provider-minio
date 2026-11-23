package minio

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
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
			{
				ResourceName:            "minio_iam_group_membership.team",
				ImportState:             true,
				ImportStateId:           groupName,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"name"},
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

func checkMinioGroupMembershipDestroyed(ctx context.Context, iamconn *madmin.AdminClient, group string) error {
	_, err := iamconn.GetGroupDescription(ctx, group)
	if err == nil {
		return fmt.Errorf("group %s still exists", group)
	}
	if !isGroupMembershipGroupMissing(err) && !strings.Contains(err.Error(), "not exist") {
		return fmt.Errorf("checking group %s after destroy: %w", group, err)
	}
	return nil
}

func testAccCheckMinioGroupMembershipDestroy(s *terraform.State) error {
	iamconn := testAccClient().S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_group_membership" {
			continue
		}

		group := rs.Primary.Attributes["group"]
		if err := checkMinioGroupMembershipDestroyed(context.Background(), iamconn, group); err != nil {
			return err
		}
	}

	return nil
}

func testAccCheckMinioGroupMembershipExists(n string, g *madmin.GroupDesc) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no User name is set")
		}

		iamconn := testAccClient().S3Admin
		gn := rs.Primary.Attributes["group"]

		resp, err := iamconn.GetGroupDescription(context.Background(), gn)
		if err != nil {
			return fmt.Errorf("error: Group (%s) not found", gn)
		}

		log.Printf("MEMBERS: %v", resp.Members)

		*g = *resp

		return nil
	}
}

func checkMinioGroupMembershipAttributes(group *madmin.GroupDesc, groupName string, users []string) error {
	if group.Name != groupName {
		return fmt.Errorf("bad group membership: expected %q, got %q", groupName, group.Name)
	}

	got := slices.Clone(group.Members)
	want := slices.Clone(users)
	slices.Sort(got)
	slices.Sort(want)

	log.Printf("MEMBERS: %v and %v", got, want)

	if !slices.Equal(got, want) {
		return fmt.Errorf("bad group membership members, expected %v, got %v", want, got)
	}
	return nil
}

func testAccCheckMinioGroupMembershipAttributes(group *madmin.GroupDesc, groupName string, users []string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		return checkMinioGroupMembershipAttributes(group, groupName, users)
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

func TestAccMinioGroupMembership_userRemovedOutsideKeepGroup(t *testing.T) {
	rString := acctest.RandString(8)
	groupName := fmt.Sprintf("tf-acc-group-gm-keep-%s", rString)
	userName := fmt.Sprintf("tf-acc-user-gm-keep-%s", rString)
	policyName := fmt.Sprintf("tf-acc-policy-gm-keep-%s", rString)
	membershipName := fmt.Sprintf("tf-acc-membership-gm-keep-%s", rString)

	var group madmin.GroupDesc
	withMembership := testAccMinioGroupMembershipWithPolicyConfig(groupName, userName, policyName, membershipName, true, true)
	withoutMembership := testAccMinioGroupMembershipWithPolicyConfig(groupName, userName, policyName, membershipName, true, false)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioGroupGone(groupName),
		Steps: []resource.TestStep{
			{
				Config: withMembership,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupMembershipExists("minio_iam_group_membership.team", &group),
					testAccCheckMinioGroupMembershipAttributes(&group, groupName, []string{userName}),
					testAccCheckMinioGroupMembershipState(groupName, policyName, []string{userName}),
				),
			},
			{
				PreConfig: func() {
					ctx := context.Background()
					admin := testAccClient().S3Admin
					if err := admin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
						Group:    groupName,
						Members:  []string{userName},
						IsRemove: true,
					}); err != nil {
						t.Fatalf("removing user %s from group %s outside Terraform: %v", userName, groupName, err)
					}
					deadline := time.Now().Add(5 * time.Second)
					for time.Now().Before(deadline) {
						desc, err := admin.GetGroupDescription(ctx, groupName)
						if err == nil && len(desc.Members) == 0 {
							return
						}
						time.Sleep(200 * time.Millisecond)
					}
					t.Fatalf("group %s still has members after outside removal", groupName)
				},
				Config: withoutMembership,
				Check:  testAccCheckMinioGroupMembershipState(groupName, policyName, []string{}),
			},
			{
				Config:   withoutMembership,
				PlanOnly: true,
			},
		},
	})
}

func TestAccMinioGroupMembership_userDeletedOutsideKeepGroup(t *testing.T) {
	rString := acctest.RandString(8)
	groupName := fmt.Sprintf("tf-acc-group-gm-delu-%s", rString)
	userName := fmt.Sprintf("tf-acc-user-gm-delu-%s", rString)
	policyName := fmt.Sprintf("tf-acc-policy-gm-delu-%s", rString)
	membershipName := fmt.Sprintf("tf-acc-membership-gm-delu-%s", rString)

	var group madmin.GroupDesc
	withMembership := testAccMinioGroupMembershipWithPolicyConfig(groupName, userName, policyName, membershipName, true, true)
	withoutMembershipAndUser := testAccMinioGroupMembershipWithPolicyConfig(groupName, userName, policyName, membershipName, false, false)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioGroupGone(groupName),
		Steps: []resource.TestStep{
			{
				Config: withMembership,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupMembershipExists("minio_iam_group_membership.team", &group),
					testAccCheckMinioGroupMembershipAttributes(&group, groupName, []string{userName}),
					testAccCheckMinioGroupMembershipState(groupName, policyName, []string{userName}),
				),
			},
			{
				PreConfig: func() {
					ctx := context.Background()
					admin := testAccClient().S3Admin
					if err := admin.RemoveUser(ctx, userName); err != nil {
						t.Fatalf("deleting user %s outside Terraform: %v", userName, err)
					}
					deadline := time.Now().Add(5 * time.Second)
					for time.Now().Before(deadline) {
						_, err := admin.GetUserInfo(ctx, userName)
						if err != nil && isGroupMembershipUserMissing(err) {
							return
						}
						time.Sleep(200 * time.Millisecond)
					}
					t.Fatalf("user %s still exists after outside deletion", userName)
				},
				Config: withoutMembershipAndUser,
				Check:  testAccCheckMinioGroupMembershipState(groupName, policyName, []string{}),
			},
			{
				Config:   withoutMembershipAndUser,
				PlanOnly: true,
			},
		},
	})
}

func testAccCheckMinioGroupMembershipState(groupName, policyName string, wantUsers []string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		desc, err := testAccClient().S3Admin.GetGroupDescription(context.Background(), groupName)
		if err != nil {
			return fmt.Errorf("reading group %s: %w", groupName, err)
		}
		if err := checkMinioGroupMembershipAttributes(desc, groupName, wantUsers); err != nil {
			return err
		}
		if !slices.Contains(strings.Split(desc.Policy, ","), policyName) {
			return fmt.Errorf("expected group %s to retain policy %q, got %q", groupName, policyName, desc.Policy)
		}
		return nil
	}
}

func testAccMinioGroupMembershipWithPolicyConfig(groupName, userName, policyName, membershipName string, includeUser, includeMembership bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, `
resource "minio_iam_group" "group" {
  name = %q
}

resource "minio_iam_policy" "policy" {
  name = %q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject"]
      Resource = ["arn:aws:s3:::*"]
    }]
  })
}

resource "minio_iam_group_policy_attachment" "attachment" {
  group_name  = minio_iam_group.group.name
  policy_name = minio_iam_policy.policy.name
}
`, groupName, policyName)

	if includeUser {
		fmt.Fprintf(&b, `
resource "minio_iam_user" "user" {
  name = %q
}
`, userName)
	}

	if includeMembership {
		fmt.Fprintf(&b, `
resource "minio_iam_group_membership" "team" {
  name  = %q
  group = minio_iam_group.group.name
  users = [minio_iam_user.user.name]
}
`, membershipName)
	}

	return b.String()
}

func TestAccMinioGroupMembership_importLifecycle(t *testing.T) {
	rString := acctest.RandString(8)
	groupName := fmt.Sprintf("tf-acc-group-gm-imp-%s", rString)
	missingGroupName := fmt.Sprintf("tf-acc-group-gm-missing-%s", rString)
	userName := fmt.Sprintf("tf-acc-user-gm-imp-%s", rString)
	policyName := fmt.Sprintf("tf-acc-policy-gm-imp-%s", rString)
	membershipName := fmt.Sprintf("tf-acc-membership-gm-imp-%s", rString)
	updatedMembershipName := fmt.Sprintf("tf-acc-membership-gm-imp-upd-%s", rString)

	var group madmin.GroupDesc
	baseOnly := testAccMinioGroupMembershipWithPolicyConfig(groupName, userName, policyName, membershipName, true, false)
	withMembership := testAccMinioGroupMembershipWithPolicyConfig(groupName, userName, policyName, membershipName, true, true)
	renamedMembership := testAccMinioGroupMembershipWithPolicyConfig(groupName, userName, policyName, updatedMembershipName, true, true)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioGroupMembershipDestroy,
		Steps: []resource.TestStep{
			{
				Config: baseOnly,
				Check:  testAccCheckMinioGroupMembershipState(groupName, policyName, []string{}),
			},
			{
				Config:        withMembership,
				ResourceName:  "minio_iam_group_membership.team",
				ImportState:   true,
				ImportStateId: missingGroupName,
				ExpectError:   regexp.MustCompile("Cannot import non-existent remote object"),
			},
			{
				PreConfig: func() {
					ctx := context.Background()
					admin := testAccClient().S3Admin
					if err := admin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{
						Group:    groupName,
						Members:  []string{userName},
						IsRemove: false,
					}); err != nil {
						t.Fatalf("adding user %s to group %s outside Terraform: %v", userName, groupName, err)
					}
					deadline := time.Now().Add(5 * time.Second)
					for time.Now().Before(deadline) {
						desc, err := admin.GetGroupDescription(ctx, groupName)
						if err == nil && slices.Contains(desc.Members, userName) {
							return
						}
						time.Sleep(200 * time.Millisecond)
					}
					t.Fatalf("group %s did not receive member %s before import", groupName, userName)
				},
				Config:             withMembership,
				ResourceName:       "minio_iam_group_membership.team",
				ImportState:        true,
				ImportStateId:      groupName,
				ImportStatePersist: true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					for _, is := range states {
						if is.Ephemeral.Type != "minio_iam_group_membership" {
							continue
						}
						if is.ID != groupName {
							return fmt.Errorf("expected imported id %q, got %q", groupName, is.ID)
						}
						if got := is.Attributes["group"]; got != groupName {
							return fmt.Errorf("expected imported group %q, got %q", groupName, got)
						}
						if got := is.Attributes["name"]; got != "" {
							return fmt.Errorf("expected imported name to be empty, got %q", got)
						}
						return nil
					}
					return fmt.Errorf("imported minio_iam_group_membership resource missing from state")
				},
			},
			{
				// Applying after import sets `name` in state in-place without replacing
				// the membership; replacement would delete the group's only member and
				// cause MinIO to drop the attached policy.
				Config: withMembership,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupMembershipExists("minio_iam_group_membership.team", &group),
					testAccCheckMinioGroupMembershipState(groupName, policyName, []string{userName}),
					resource.TestCheckResourceAttr("minio_iam_group_membership.team", "id", groupName),
					resource.TestCheckResourceAttr("minio_iam_group_membership.team", "name", membershipName),
				),
			},
			{
				Config: renamedMembership,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupMembershipExists("minio_iam_group_membership.team", &group),
					testAccCheckMinioGroupMembershipState(groupName, policyName, []string{userName}),
					resource.TestCheckResourceAttr("minio_iam_group_membership.team", "id", groupName),
					resource.TestCheckResourceAttr("minio_iam_group_membership.team", "name", updatedMembershipName),
				),
			},
		},
	})
}

func TestAccMinioGroupMembership_upgradeFromLegacyStateID(t *testing.T) {
	t.Setenv("TF_ACC_PROVIDER_NAMESPACE", "aminueza")

	rString := acctest.RandString(8)
	groupName := fmt.Sprintf("tf-acc-group-gm-upg-%s", rString)
	userName := fmt.Sprintf("tf-acc-user-gm-upg-%s", rString)
	membershipName := fmt.Sprintf("tf-acc-membership-gm-upg-%s", rString)

	var group madmin.GroupDesc
	legacyCfg := testAccMinioGroupMemberConfig(groupName, userName, membershipName)
	upgradedCfg := fmt.Sprintf(`
terraform {
  required_providers {
    minio = {
      source = "aminueza/minio"
    }
  }
}

provider "minio" {}
%s`, legacyCfg)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		CheckDestroy: testAccCheckMinioGroupMembershipDestroy,
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"minio": {
						Source:            "aminueza/minio",
						VersionConstraint: "3.46.4",
					},
				},
				Config: legacyCfg,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupMembershipExists("minio_iam_group_membership.team", &group),
					resource.TestCheckResourceAttr("minio_iam_group_membership.team", "id", membershipName),
				),
			},
			{
				ProviderFactories:  testAccProviders,
				Config:             upgradedCfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				ProviderFactories: testAccProviders,
				Config:            upgradedCfg,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupMembershipExists("minio_iam_group_membership.team", &group),
					resource.TestCheckResourceAttr("minio_iam_group_membership.team", "id", groupName),
					resource.TestCheckResourceAttr("minio_iam_group_membership.team", "name", membershipName),
				),
			},
		},
	})
}
