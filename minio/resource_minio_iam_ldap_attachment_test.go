package minio

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func testAccLDAPAttachmentPreCheck(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)

	if os.Getenv("MINIO_LDAP_ENABLED") != "1" {
		t.Skip("Skipping LDAP tests: set MINIO_LDAP_ENABLED=1 to run")
	}
	if os.Getenv("MINIO_LDAP_TEST_USER_DN") == "" {
		t.Skip("Skipping LDAP tests: MINIO_LDAP_TEST_USER_DN not set")
	}
}

func TestAccMinioIAMLDAPUserPolicyAttachment_basic(t *testing.T) {
	userDN := os.Getenv("MINIO_LDAP_TEST_USER_DN")
	resourceName := "minio_iam_ldap_user_policy_attachment.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccLDAPAttachmentPreCheck(t) },
		ProviderFactories: testAccProviders,
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
		ProviderFactories: testAccProviders,
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

func testAccLDAPUserPolicyAttachmentConfig(userDN, policyName string) string {
	return fmt.Sprintf(`
resource "minio_iam_ldap_user_policy_attachment" "test" {
  user_dn     = %[1]q
  policy_name = %[2]q
}
`, userDN, policyName)
}

func testAccLDAPGroupPolicyAttachmentConfig(groupDN, policyName string) string {
	return fmt.Sprintf(`
resource "minio_iam_ldap_group_policy_attachment" "test" {
  group_dn    = %[1]q
  policy_name = %[2]q
}
`, groupDN, policyName)
}
