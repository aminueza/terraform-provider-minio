package minio

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v3"
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
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:      testAccCheckLDAPUserPolicyAttachmentDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccLDAPUserPolicyAttachmentConfig(userDN, "readonly"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "user_dn", userDN),
					resource.TestCheckResourceAttr(resourceName, "policy_name", "readonly"),
				),
			},
		},
	})
}

func TestAccMinioIAMLDAPGroupPolicyAttachment_basic(t *testing.T) {
	groupDN := os.Getenv("MINIO_LDAP_TEST_GROUP_DN")
	resourceName := "minio_iam_ldap_group_policy_attachment.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccLDAPAttachmentPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:      testAccCheckLDAPGroupPolicyAttachmentDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccLDAPGroupPolicyAttachmentConfig(groupDN, "readonly"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "group_dn", groupDN),
					resource.TestCheckResourceAttr(resourceName, "policy_name", "readonly"),
				),
			},
		},
	})
}

func testAccCheckLDAPUserPolicyAttachmentDestroy(s *terraform.State) error {
	client := testMustGetMinioClientWithPrefix("LDAP_").S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_ldap_user_policy_attachment" {
			continue
		}

		userDN := rs.Primary.Attributes["user_dn"]
		policyName := rs.Primary.Attributes["policy_name"]

		per, err := client.GetLDAPPolicyEntities(context.Background(), madmin.PolicyEntitiesQuery{
			Policy: []string{policyName},
			Users:  []string{userDN},
		})
		if err != nil {
			continue
		}
		if len(per.PolicyMappings) > 0 {
			return fmt.Errorf("LDAP user policy attachment %s/%s still exists", userDN, policyName)
		}
	}

	return nil
}

func testAccCheckLDAPGroupPolicyAttachmentDestroy(s *terraform.State) error {
	client := testMustGetMinioClientWithPrefix("LDAP_").S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_ldap_group_policy_attachment" {
			continue
		}

		groupDN := rs.Primary.Attributes["group_dn"]
		policyName := rs.Primary.Attributes["policy_name"]

		per, err := client.GetLDAPPolicyEntities(context.Background(), madmin.PolicyEntitiesQuery{
			Policy: []string{policyName},
			Groups: []string{groupDN},
		})
		if err != nil {
			continue
		}
		if len(per.PolicyMappings) > 0 {
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
