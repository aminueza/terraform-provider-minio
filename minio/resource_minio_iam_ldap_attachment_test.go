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
	if os.Getenv("LDAP_MINIO_ENDPOINT") == "" {
		t.Skip("Skipping LDAP tests: LDAP_MINIO_ENDPOINT not set")
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
provider "minio" {
  alias          = "ldap"
  minio_server   = "%s"
  minio_user     = "%s"
  minio_password = "%s"
  minio_ssl      = false
}

resource "minio_iam_ldap_user_policy_attachment" "test" {
  provider    = minio.ldap
  user_dn     = %[4]q
  policy_name = %[5]q
}
`, os.Getenv("LDAP_MINIO_ENDPOINT"), os.Getenv("LDAP_MINIO_USER"), os.Getenv("LDAP_MINIO_PASSWORD"), userDN, policyName)
}

func testAccLDAPGroupPolicyAttachmentConfig(groupDN, policyName string) string {
	return fmt.Sprintf(`
provider "minio" {
  alias          = "ldap"
  minio_server   = "%s"
  minio_user     = "%s"
  minio_password = "%s"
  minio_ssl      = false
}

resource "minio_iam_ldap_group_policy_attachment" "test" {
  provider    = minio.ldap
  group_dn    = %[4]q
  policy_name = %[5]q
}
`, os.Getenv("LDAP_MINIO_ENDPOINT"), os.Getenv("LDAP_MINIO_USER"), os.Getenv("LDAP_MINIO_PASSWORD"), groupDN, policyName)
}
