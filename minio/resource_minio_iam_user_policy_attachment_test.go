package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccMinioIAMUserPolicyAttachment_basic(t *testing.T) {
	userName := "tfacc-user-" + acctest.RandString(8)
	policyName := "tfacc-policy-" + acctest.RandString(8)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMUserPolicyAttachmentConfig(userName, policyName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("minio_iam_user_policy_attachment.test", "id"),
					resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test", "user_name", userName),
					resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test", "policy_name", policyName),
				),
			},
		},
	})
}

func TestAccMinioIAMUserPolicyAttachment_multiplePolices(t *testing.T) {
	userName := "tfacc-user-" + acctest.RandString(8)
	policy1Name := "tfacc-policy1-" + acctest.RandString(8)
	policy2Name := "tfacc-policy2-" + acctest.RandString(8)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMUserPolicyAttachmentConfigMultiple(userName, policy1Name, policy2Name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test1", "user_name", userName),
					resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test1", "policy_name", policy1Name),
					resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test2", "user_name", userName),
					resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test2", "policy_name", policy2Name),
				),
			},
		},
	})
}

func TestAccMinioIAMUserPolicyAttachment_update(t *testing.T) {
	userName := "tfacc-user-" + acctest.RandString(8)
	policy1Name := "tfacc-policy1-" + acctest.RandString(8)
	policy2Name := "tfacc-policy2-" + acctest.RandString(8)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMUserPolicyAttachmentConfig(userName, policy1Name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test", "policy_name", policy1Name),
				),
			},
			{
				Config: testAccMinioIAMUserPolicyAttachmentConfig(userName, policy2Name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("minio_iam_user_policy_attachment.test", "policy_name", policy2Name),
				),
			},
		},
	})
}

func testAccMinioIAMUserPolicyAttachmentConfig(userName, policyName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test" {
  name   = %[1]q
  secret = "Test123456"
}

resource "minio_iam_policy" "test" {
  name   = %[2]q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject"]
      Resource = ["arn:aws:s3:::test-bucket/*"]
    }]
  })
}

resource "minio_iam_user_policy_attachment" "test" {
  user_name   = minio_iam_user.test.name
  policy_name = minio_iam_policy.test.name
}
`, userName, policyName)
}

func testAccMinioIAMUserPolicyAttachmentConfigMultiple(userName, policy1Name, policy2Name string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test" {
  name   = %[1]q
  secret = "Test123456"
}

resource "minio_iam_policy" "test1" {
  name   = %[2]q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject"]
      Resource = ["arn:aws:s3:::test-bucket/*"]
    }]
  })
}

resource "minio_iam_policy" "test2" {
  name   = %[3]q
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:PutObject"]
      Resource = ["arn:aws:s3:::test-bucket/*"]
    }]
  })
}

resource "minio_iam_user_policy_attachment" "test1" {
  user_name   = minio_iam_user.test.name
  policy_name = minio_iam_policy.test1.name
}

resource "minio_iam_user_policy_attachment" "test2" {
  user_name   = minio_iam_user.test.name
  policy_name = minio_iam_policy.test2.name
}
`, userName, policy1Name, policy2Name)
}
