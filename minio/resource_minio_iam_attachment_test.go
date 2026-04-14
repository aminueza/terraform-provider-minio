package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioIAMGroupPolicyAttachment_basic(t *testing.T) {
	groupName := "tfacc-grp-pol-" + acctest.RandString(6)
	policyName := "tfacc-pol-" + acctest.RandString(6)
	resourceName := "minio_iam_group_policy_attachment.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMGroupPolicyAttachmentConfig(groupName, policyName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "group_name", groupName),
					resource.TestCheckResourceAttr(resourceName, "policy_name", policyName),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateId:           fmt.Sprintf("%s/%s", groupName, policyName),
				ImportStateVerify:       false,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					s := states[0]
					if s.Attributes["group_name"] != groupName {
						return fmt.Errorf("expected group_name %q, got %q", groupName, s.Attributes["group_name"])
					}
					if s.Attributes["policy_name"] != policyName {
						return fmt.Errorf("expected policy_name %q, got %q", policyName, s.Attributes["policy_name"])
					}
					return nil
				},
			},
		},
	})
}

func TestAccMinioIAMUserPolicyAttachment_basic(t *testing.T) {
	userName := "tfacc-usr-pol-" + acctest.RandString(6)
	policyName := "tfacc-pol-" + acctest.RandString(6)
	resourceName := "minio_iam_user_policy_attachment.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMUserPolicyAttachmentConfig(userName, policyName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "user_name", userName),
					resource.TestCheckResourceAttr(resourceName, "policy_name", policyName),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateId:           fmt.Sprintf("%s/%s", userName, policyName),
				ImportStateVerify:       false,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					s := states[0]
					if s.Attributes["user_name"] != userName {
						return fmt.Errorf("expected user_name %q, got %q", userName, s.Attributes["user_name"])
					}
					if s.Attributes["policy_name"] != policyName {
						return fmt.Errorf("expected policy_name %q, got %q", policyName, s.Attributes["policy_name"])
					}
					return nil
				},
			},
		},
	})
}

func TestAccMinioIAMGroupUserAttachment_basic(t *testing.T) {
	groupName := "tfacc-grp-usr-" + acctest.RandString(6)
	userName := "tfacc-usr-att-" + acctest.RandString(6)
	resourceName := "minio_iam_group_user_attachment.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMGroupUserAttachmentConfig(groupName, userName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "group_name", groupName),
					resource.TestCheckResourceAttr(resourceName, "user_name", userName),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateId:           fmt.Sprintf("%s/%s", groupName, userName),
				ImportStateVerify:       false,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					s := states[0]
					if s.Attributes["group_name"] != groupName {
						return fmt.Errorf("expected group_name %q, got %q", groupName, s.Attributes["group_name"])
					}
					if s.Attributes["user_name"] != userName {
						return fmt.Errorf("expected user_name %q, got %q", userName, s.Attributes["user_name"])
					}
					return nil
				},
			},
		},
	})
}

func TestAccMinioIAMUserPolicyAttachment_detach(t *testing.T) {
	userName := "tfacc-usr-detach-" + acctest.RandString(6)
	policyName := "tfacc-pol-detach-" + acctest.RandString(6)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
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
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
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
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
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
