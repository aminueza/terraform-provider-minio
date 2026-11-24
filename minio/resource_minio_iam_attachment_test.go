package minio

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccMinioIAMGroupPolicyAttachment_basic(t *testing.T) {
	groupName := "tfacc-grp-pol-" + acctest.RandString(6)
	policyName := "tfacc-pol-" + acctest.RandString(6)
	resourceName := "minio_iam_group_policy_attachment.test"
	expectedID := fmt.Sprintf("%s/%s", groupName, policyName)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMGroupPolicyAttachmentConfig(groupName, policyName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", expectedID),
					resource.TestCheckResourceAttr(resourceName, "group_name", groupName),
					resource.TestCheckResourceAttr(resourceName, "policy_name", policyName),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccMinioIAMUserPolicyAttachment_basic(t *testing.T) {
	userName := "tfacc-usr-pol-" + acctest.RandString(6)
	policyName := "tfacc-pol-" + acctest.RandString(6)
	resourceName := "minio_iam_user_policy_attachment.test"
	expectedID := fmt.Sprintf("%s/%s", userName, policyName)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMUserPolicyAttachmentConfig(userName, policyName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", expectedID),
					resource.TestCheckResourceAttr(resourceName, "user_name", userName),
					resource.TestCheckResourceAttr(resourceName, "policy_name", policyName),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccMinioIAMGroupUserAttachment_basic(t *testing.T) {
	groupName := "tfacc-grp-usr-" + acctest.RandString(6)
	userName := "tfacc-usr-att-" + acctest.RandString(6)
	resourceName := "minio_iam_group_user_attachment.test"
	expectedID := fmt.Sprintf("%s/%s", groupName, userName)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMGroupUserAttachmentConfig(groupName, userName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", expectedID),
					resource.TestCheckResourceAttr(resourceName, "group_name", groupName),
					resource.TestCheckResourceAttr(resourceName, "user_name", userName),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccMinioIAMUserPolicyAttachment_detach(t *testing.T) {
	userName := "tfacc-usr-detach-" + acctest.RandString(6)
	policyName := "tfacc-pol-detach-" + acctest.RandString(6)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMUserPolicyAttachmentConfig(userName, policyName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test", "user_name", userName),
					resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test", "policy_name", policyName),
				),
			},
			{
				Config: testAccIAMUserNoPolicyConfig(userName, policyName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_user.test", "name", userName),
				),
			},
		},
	})
}

func TestAccMinioIAMGroupPolicyAttachment_detach(t *testing.T) {
	groupName := "tfacc-grp-detach-" + acctest.RandString(6)
	policyName := "tfacc-pol-detach-" + acctest.RandString(6)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMGroupPolicyAttachmentConfig(groupName, policyName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_group_policy_attachment.test", "group_name", groupName),
					resource.TestCheckResourceAttr("minio_iam_group_policy_attachment.test", "policy_name", policyName),
				),
			},
			{
				Config: testAccIAMGroupNoPolicyConfig(groupName, policyName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_group.test", "name", groupName),
				),
			},
		},
	})
}

func TestAccMinioIAMUserPolicyAttachment_multiplePolicies(t *testing.T) {
	userName := "tfacc-usr-multi-" + acctest.RandString(6)
	policy1 := "tfacc-pol1-" + acctest.RandString(6)
	policy2 := "tfacc-pol2-" + acctest.RandString(6)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMUserMultiplePoliciesConfig(userName, policy1, policy2),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.pol1", "policy_name", policy1),
					resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.pol2", "policy_name", policy2),
				),
			},
			{
				Config: testAccIAMUserSinglePolicyConfig(userName, policy1, policy2),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.pol1", "policy_name", policy1),
				),
			},
		},
	})
}

func testAccIAMUserNoPolicyConfig(userName, policyName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test" {
  name = %[1]q
}

resource "minio_iam_policy" "test" {
  name   = %[2]q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject"]
      Resource = ["arn:aws:s3:::*"]
    }]
  })
}
`, userName, policyName)
}

func testAccIAMGroupNoPolicyConfig(groupName, policyName string) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "test" {
  name = %[1]q
}

resource "minio_iam_policy" "test" {
  name   = %[2]q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject"]
      Resource = ["arn:aws:s3:::*"]
    }]
  })
}
`, groupName, policyName)
}

func testAccIAMUserMultiplePoliciesConfig(userName, policy1, policy2 string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test" {
  name = %[1]q
}

resource "minio_iam_policy" "pol1" {
  name   = %[2]q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{ Effect = "Allow", Action = ["s3:GetObject"], Resource = ["arn:aws:s3:::*"] }]
  })
}

resource "minio_iam_policy" "pol2" {
  name   = %[3]q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{ Effect = "Allow", Action = ["s3:PutObject"], Resource = ["arn:aws:s3:::*"] }]
  })
}

resource "minio_iam_user_policy_attachment" "pol1" {
  user_name   = minio_iam_user.test.name
  policy_name = minio_iam_policy.pol1.name
}

resource "minio_iam_user_policy_attachment" "pol2" {
  user_name   = minio_iam_user.test.name
  policy_name = minio_iam_policy.pol2.name
}
`, userName, policy1, policy2)
}

func testAccIAMUserSinglePolicyConfig(userName, policy1, policy2 string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test" {
  name = %[1]q
}

resource "minio_iam_policy" "pol1" {
  name   = %[2]q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{ Effect = "Allow", Action = ["s3:GetObject"], Resource = ["arn:aws:s3:::*"] }]
  })
}

resource "minio_iam_policy" "pol2" {
  name   = %[3]q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{ Effect = "Allow", Action = ["s3:PutObject"], Resource = ["arn:aws:s3:::*"] }]
  })
}

resource "minio_iam_user_policy_attachment" "pol1" {
  user_name   = minio_iam_user.test.name
  policy_name = minio_iam_policy.pol1.name
}
`, userName, policy1, policy2)
}

func testAccIAMGroupPolicyAttachmentConfig(groupName, policyName string) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "test" {
  name = %[1]q
}

resource "minio_iam_policy" "test" {
  name   = %[2]q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject"]
      Resource = ["arn:aws:s3:::*"]
    }]
  })
}

resource "minio_iam_group_policy_attachment" "test" {
  group_name  = minio_iam_group.test.name
  policy_name = minio_iam_policy.test.name
}
`, groupName, policyName)
}

func testAccIAMUserPolicyAttachmentConfig(userName, policyName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test" {
  name = %[1]q
}

resource "minio_iam_policy" "test" {
  name   = %[2]q
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
`, userName, policyName)
}

func testAccIAMGroupUserAttachmentConfig(groupName, userName string) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "test" {
  name = %[1]q
}

resource "minio_iam_user" "test" {
  name = %[2]q
}

resource "minio_iam_group_user_attachment" "test" {
  group_name = minio_iam_group.test.name
  user_name  = minio_iam_user.test.name
}
`, groupName, userName)
}

func TestAccMinioIAMAttachments_longNamesAndValidation(t *testing.T) {
	suffix := acctest.RandString(6)
	longPolicy := "tfacc-pol-" + suffix + "-" + strings.Repeat("p", 100-len("tfacc-pol-"+suffix+"-"))
	longGroup := "tfacc-grp-" + suffix + "-" + strings.Repeat("g", 100-len("tfacc-grp-"+suffix+"-"))
	groupName := "tfacc-grp-a-" + suffix
	userName := "tfacc-usr-a-" + suffix

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:      testAccIAMGroupPolicyAttachmentConfig(groupName, "a,b"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`must\s+match`),
			},
			{
				Config:      testAccIAMUserPolicyAttachmentConfig(userName, "a,b"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`must\s+match`),
			},
			{
				Config: `
resource "minio_iam_ldap_group_policy_attachment" "test" {
  group_dn    = "CN=admins,OU=Groups,DC=example,DC=com"
  policy_name = "a,b"
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`must\s+match`),
			},
			{
				Config: `
resource "minio_iam_ldap_user_policy_attachment" "test" {
  user_dn     = "CN=alice,OU=Users,DC=example,DC=com"
  policy_name = "a,b"
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`must\s+match`),
			},
			{
				Config: testAccIAMGroupPolicyAttachmentConfig(groupName, longPolicy),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_group_policy_attachment.test", "policy_name", longPolicy),
				),
			},
			{
				Config: testAccIAMUserPolicyAttachmentConfig(userName, longPolicy),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test", "policy_name", longPolicy),
				),
			},
			{
				Config: testAccIAMGroupUserAttachmentConfig(longGroup, userName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_group_user_attachment.test", "group_name", longGroup),
				),
			},
		},
	})
}

func TestAccMinioIAMGroupUserAttachment_groupDeletedOutsideTerraform(t *testing.T) {
	rString := acctest.RandString(8)
	groupName := fmt.Sprintf("tf-acc-group-del-%s", rString)
	userName := fmt.Sprintf("tf-acc-user-del-%s", rString)
	config := testAccIAMGroupUserAttachmentConfig(groupName, userName)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioGroupGone(groupName),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_group_user_attachment.test", "group_name", groupName),
					testAccCheckMinioGroupMemberExists(groupName, userName),
				),
			},
			{
				PreConfig:          func() { testAccDeleteMinioGroup(t, groupName) },
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_group_user_attachment.test", "group_name", groupName),
					testAccCheckMinioGroupMemberExists(groupName, userName),
				),
			},
		},
	})
}

func TestAccMinioIAMGroupPolicyAttachment_groupDeletedOutsideTerraform(t *testing.T) {
	rString := acctest.RandString(8)
	groupName := fmt.Sprintf("tf-acc-group-pol-del-%s", rString)
	policyName := fmt.Sprintf("tf-acc-policy-del-%s", rString)
	config := testAccIAMGroupPolicyAttachmentConfig(groupName, policyName)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioGroupGone(groupName),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_group_policy_attachment.test", "group_name", groupName),
					testAccCheckMinioGroupPolicyAttached(groupName, policyName),
				),
			},
			{
				PreConfig:          func() { testAccDeleteMinioGroup(t, groupName) },
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_group_policy_attachment.test", "group_name", groupName),
					testAccCheckMinioGroupPolicyAttached(groupName, policyName),
				),
			},
		},
	})
}

func TestAccMinioIAMUserPolicyAttachment_upgradeFromOldID(t *testing.T) {
	userName := "tfacc-usr-upg-" + acctest.RandString(6)
	policyName := "tfacc-pol-upg-" + acctest.RandString(6)
	resourceName := "minio_iam_user_policy_attachment.test"
	expectedID := fmt.Sprintf("%s/%s", userName, policyName)
	cfg := testAccIAMUserPolicyAttachmentConfig(userName, policyName)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		CheckDestroy: testAccCheckMinioIAMPolicyDestroy,
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"minio": {
						Source:            "aminueza/minio",
						VersionConstraint: "3.46.4",
					},
				},
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(resourceName, "id", regexp.MustCompile("^"+regexp.QuoteMeta(userName)+"-")),
				),
			},
			{
				ProviderFactories:  testAccProviders,
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				ProviderFactories: testAccProviders,
				Config:            cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", expectedID),
				),
			},
		},
	})
}

func TestAccMinioIAMGroupPolicyAttachment_upgradeFromOldID(t *testing.T) {
	groupName := "tfacc-grp-upg-" + acctest.RandString(6)
	policyName := "tfacc-pol-upg-" + acctest.RandString(6)
	resourceName := "minio_iam_group_policy_attachment.test"
	expectedID := fmt.Sprintf("%s/%s", groupName, policyName)
	cfg := testAccIAMGroupPolicyAttachmentConfig(groupName, policyName)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		CheckDestroy: testAccCheckMinioIAMPolicyDestroy,
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"minio": {
						Source:            "aminueza/minio",
						VersionConstraint: "3.46.4",
					},
				},
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(resourceName, "id", regexp.MustCompile("^"+regexp.QuoteMeta(groupName)+"-")),
				),
			},
			{
				ProviderFactories:  testAccProviders,
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				ProviderFactories: testAccProviders,
				Config:            cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", expectedID),
				),
			},
		},
	})
}

func TestAccMinioIAMGroupUserAttachment_upgradeFromOldID(t *testing.T) {
	groupName := "tfacc-grp-upg-" + acctest.RandString(6)
	userName := "tfacc-usr-upg-" + acctest.RandString(6)
	resourceName := "minio_iam_group_user_attachment.test"
	expectedID := fmt.Sprintf("%s/%s", groupName, userName)
	cfg := testAccIAMGroupUserAttachmentConfig(groupName, userName)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"minio": {
						Source:            "aminueza/minio",
						VersionConstraint: "3.46.4",
					},
				},
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(resourceName, "id", regexp.MustCompile("^"+regexp.QuoteMeta(expectedID)+"[0-9a-f]+$")),
				),
			},
			{
				ProviderFactories:  testAccProviders,
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				ProviderFactories: testAccProviders,
				Config:            cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", expectedID),
				),
			},
		},
	})
}
