package minio

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
)

func testAccLDAPAttachmentPreCheck(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)

	if os.Getenv("MINIO_LDAP_ENABLED") != "1" {
		t.Skip("Skipping LDAP tests: set MINIO_LDAP_ENABLED=1 to run")
	}
	if os.Getenv("LDAP_MINIO_ENDPOINT") == "" {
		t.Skip("Skipping LDAP tests: LDAP_MINIO_ENDPOINT not set")
	}
	if os.Getenv("MINIO_LDAP_TEST_USER_DN") == "" {
		t.Skip("Skipping LDAP tests: MINIO_LDAP_TEST_USER_DN not set")
	}
	if os.Getenv("MINIO_LDAP_TEST_GROUP_DN") == "" {
		t.Skip("Skipping LDAP tests: MINIO_LDAP_TEST_GROUP_DN not set")
	}
}

func TestAccMinioIAMLDAPUserPolicyAttachment_basic(t *testing.T) {
	userDN := os.Getenv("MINIO_LDAP_TEST_USER_DN")
	resourceName := "minio_iam_ldap_user_policy_attachment.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccLDAPAttachmentPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckLDAPUserPolicyAttachmentDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccLDAPUserPolicyAttachmentConfig(userDN, "readonly"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", fmt.Sprintf("%s/readonly", userDN)),
					resource.TestCheckResourceAttr(resourceName, "user_dn", userDN),
					resource.TestCheckResourceAttr(resourceName, "policy_name", "readonly"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateId:     fmt.Sprintf("%s/readonly", userDN),
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccMinioIAMLDAPGroupPolicyAttachment_basic(t *testing.T) {
	groupDN := os.Getenv("MINIO_LDAP_TEST_GROUP_DN")
	resourceName := "minio_iam_ldap_group_policy_attachment.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccLDAPAttachmentPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckLDAPGroupPolicyAttachmentDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccLDAPGroupPolicyAttachmentConfig(groupDN, "readonly"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", fmt.Sprintf("%s/readonly", groupDN)),
					resource.TestCheckResourceAttr(resourceName, "group_dn", groupDN),
					resource.TestCheckResourceAttr(resourceName, "policy_name", "readonly"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateId:     fmt.Sprintf("%s/readonly", groupDN),
				ImportStateVerify: true,
			},
		},
	})
}

// The user and the group share the policy, so MinIO keeps listing it for the
// queried policy after one of them loses it. Read has to notice anyway.
func TestAccMinioIAMLDAPPolicyAttachment_detachedOutsideTerraform(t *testing.T) {
	userDN := os.Getenv("MINIO_LDAP_TEST_USER_DN")
	groupDN := os.Getenv("MINIO_LDAP_TEST_GROUP_DN")
	policyName := "writeonly"
	config := testAccLDAPUserPolicyAttachmentConfig(userDN, policyName) + testAccLDAPGroupPolicyAttachmentConfig(groupDN, policyName)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccLDAPAttachmentPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy: resource.ComposeTestCheckFunc(
			testAccCheckLDAPUserPolicyAttachmentDestroy,
			testAccCheckLDAPGroupPolicyAttachmentDestroy,
		),
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			{
				PreConfig: func() {
					testAccDetachLDAPPolicy(t, madmin.PolicyAssociationReq{Policies: []string{policyName}, User: userDN})
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				Check:  testAccCheckLDAPPolicyAttached(userDN, "", policyName),
			},
			{
				PreConfig: func() {
					testAccDetachLDAPPolicy(t, madmin.PolicyAssociationReq{Policies: []string{policyName}, Group: groupDN})
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				Check:  testAccCheckLDAPPolicyAttached("", groupDN, policyName),
			},
		},
	})
}

func testAccDetachLDAPPolicy(t *testing.T, req madmin.PolicyAssociationReq) {
	t.Helper()
	ldap, err := testAccLdapClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ldap.S3Admin.DetachPolicyLDAP(context.Background(), req); err != nil {
		t.Fatalf("detaching %v outside Terraform: %v", req, err)
	}
}

func testAccCheckLDAPPolicyAttached(userDN, groupDN, policyName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ldap, err := testAccLdapClient()
		if err != nil {
			return err
		}
		var policies []string
		var diags diag.Diagnostics
		if userDN != "" {
			policies, diags = minioReadLDAPUserPolicies(context.Background(), ldap.S3Admin, userDN)
		} else {
			policies, diags = minioReadLDAPGroupPolicies(context.Background(), ldap.S3Admin, groupDN)
		}
		if diags.HasError() {
			return fmt.Errorf("reading policies of %s%s: %v", userDN, groupDN, diags)
		}
		if !Contains(policies, policyName) {
			return fmt.Errorf("%s%s has policies %v, want %s attached again", userDN, groupDN, policies, policyName)
		}
		return nil
	}
}

func testAccCheckLDAPUserPolicyAttachmentDestroy(s *terraform.State) error {
	ldap, err := testAccLdapClient()
	if err != nil {
		return err
	}
	client := ldap.S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_ldap_user_policy_attachment" {
			continue
		}

		userDN := rs.Primary.Attributes["user_dn"]
		policyName := rs.Primary.Attributes["policy_name"]

		policies, diags := minioReadLDAPUserPolicies(context.Background(), client, userDN)
		if diags.HasError() {
			return fmt.Errorf("reading policies of %s: %v", userDN, diags)
		}
		if Contains(policies, policyName) {
			return fmt.Errorf("LDAP user policy attachment %s/%s still exists", userDN, policyName)
		}
	}

	return nil
}

func testAccCheckLDAPGroupPolicyAttachmentDestroy(s *terraform.State) error {
	ldap, err := testAccLdapClient()
	if err != nil {
		return err
	}
	client := ldap.S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_ldap_group_policy_attachment" {
			continue
		}

		groupDN := rs.Primary.Attributes["group_dn"]
		policyName := rs.Primary.Attributes["policy_name"]

		policies, diags := minioReadLDAPGroupPolicies(context.Background(), client, groupDN)
		if diags.HasError() {
			return fmt.Errorf("reading policies of %s: %v", groupDN, diags)
		}
		if Contains(policies, policyName) {
			return fmt.Errorf("LDAP group policy attachment %s/%s still exists", groupDN, policyName)
		}
	}

	return nil
}

func testAccLDAPUserPolicyAttachmentConfig(userDN, policyName string) string {
	return fmt.Sprintf(`
resource "minio_iam_ldap_user_policy_attachment" "test" {
  provider    = ldapminio
  user_dn     = %[1]q
  policy_name = %[2]q
}
`, userDN, policyName)
}

func testAccLDAPGroupPolicyAttachmentConfig(groupDN, policyName string) string {
	return fmt.Sprintf(`
resource "minio_iam_ldap_group_policy_attachment" "test" {
  provider    = ldapminio
  group_dn    = %[1]q
  policy_name = %[2]q
}
`, groupDN, policyName)
}

func TestAccMinioIAMLDAPPolicyAttachment_upgradeLegacyStateID(t *testing.T) {
	t.Setenv("TF_ACC_PROVIDER_NAMESPACE", "aminueza")

	userDN := os.Getenv("MINIO_LDAP_TEST_USER_DN")
	groupDN := os.Getenv("MINIO_LDAP_TEST_GROUP_DN")
	ldapServer := os.Getenv("LDAP_MINIO_ENDPOINT")
	ldapUser := os.Getenv("LDAP_MINIO_USER")
	ldapPassword := os.Getenv("LDAP_MINIO_PASSWORD")
	ldapSSL, _ := strconv.ParseBool(os.Getenv("LDAP_MINIO_ENABLE_HTTPS"))

	legacyCfg := fmt.Sprintf(`
provider "minio" {
  minio_server   = %[1]q
  minio_user     = %[2]q
  minio_password = %[3]q
  minio_ssl      = %[4]t
}

resource "minio_iam_ldap_user_policy_attachment" "test" {
  user_dn     = %[5]q
  policy_name = "readonly"
}

resource "minio_iam_ldap_group_policy_attachment" "test" {
  group_dn    = %[6]q
  policy_name = "readonly"
}
`, ldapServer, ldapUser, ldapPassword, ldapSSL, userDN, groupDN)

	upgradedCfg := fmt.Sprintf(`
terraform {
  required_providers {
    minio = {
      source = "aminueza/minio"
    }
  }
}
%s`, legacyCfg)

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccLDAPAttachmentPreCheck(t) },
		CheckDestroy: resource.ComposeTestCheckFunc(
			testAccCheckLDAPUserPolicyAttachmentDestroy,
			testAccCheckLDAPGroupPolicyAttachmentDestroy,
		),
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
					resource.TestCheckResourceAttr("minio_iam_ldap_user_policy_attachment.test", "id", fmt.Sprintf("readonly/%s", userDN)),
					resource.TestCheckResourceAttr("minio_iam_ldap_group_policy_attachment.test", "id", fmt.Sprintf("readonly/%s", groupDN)),
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
					resource.TestCheckResourceAttr("minio_iam_ldap_user_policy_attachment.test", "id", fmt.Sprintf("%s/readonly", userDN)),
					resource.TestCheckResourceAttr("minio_iam_ldap_group_policy_attachment.test", "id", fmt.Sprintf("%s/readonly", groupDN)),
				),
			},
		},
	})
}
