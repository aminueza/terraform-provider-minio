package minio

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioIAMGroupPolicy_basic(t *testing.T) {
	var groupPolicy1, groupPolicy2 string
	rInt := acctest.RandInt()
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckIAMGroupPolicyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMGroupPolicyConfig(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIAMGroupPolicyExists(
						"minio_iam_group.group",
						"minio_iam_group_policy.foo",
						&groupPolicy1,
					),
				),
			},
			{
				ResourceName:      "minio_iam_group_policy.foo",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccIAMGroupPolicyConfigUpdate(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIAMGroupPolicyExists(
						"minio_iam_group.group",
						"minio_iam_group_policy.bar",
						&groupPolicy2,
					),
					testAccCheckMinioIAMGroupPolicyNameChanged(&groupPolicy1, &groupPolicy2),
				),
			},
			{
				ResourceName:      "minio_iam_group_policy.bar",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccMinioIAMGroupPolicy_disappears(t *testing.T) {
	var out string
	rInt := acctest.RandInt()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckIAMGroupPolicyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMGroupPolicyConfig(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIAMGroupPolicyExists(
						"minio_iam_group.group",
						"minio_iam_group_policy.foo",
						&out,
					),
					testAccCheckIAMGroupPolicyDisappears("minio_iam_group_policy.foo"),
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccMinioIAMGroupPolicy_policyContentUpdate(t *testing.T) {
	var groupPolicy string
	rInt := acctest.RandInt()
	resourceName := "minio_iam_group_policy.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckIAMGroupPolicyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMGroupPolicyConfigWithAction(rInt, "s3:ListBucket"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIAMGroupPolicyExists(
						"minio_iam_group.test",
						resourceName,
						&groupPolicy,
					),
					resource.TestCheckResourceAttr(resourceName, "name", fmt.Sprintf("test_policy_%d", rInt)),
					testCheckResourceAttrContains(resourceName, "policy", "s3:ListBucket"),
				),
			},
			{
				Config: testAccIAMGroupPolicyConfigWithAction(rInt, "s3:GetObject"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIAMGroupPolicyExists(
						"minio_iam_group.test",
						resourceName,
						&groupPolicy,
					),
					resource.TestCheckResourceAttr(resourceName, "name", fmt.Sprintf("test_policy_%d", rInt)),
					testCheckResourceAttrContains(resourceName, "policy", "s3:GetObject"),
				),
			},
			{
				Config: testAccIAMGroupPolicyConfigWithMultipleActions(rInt),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIAMGroupPolicyExists(
						"minio_iam_group.test",
						resourceName,
						&groupPolicy,
					),
					resource.TestCheckResourceAttr(resourceName, "name", fmt.Sprintf("test_policy_%d", rInt)),
					testCheckResourceAttrContains(resourceName, "policy", "s3:GetObject"),
					testCheckResourceAttrContains(resourceName, "policy", "s3:PutObject"),
					testCheckResourceAttrContains(resourceName, "policy", "s3:DeleteObject"),
				),
			},
		},
	})
}

func TestAccMinioIAMGroupPolicy_namePrefix(t *testing.T) {
	var groupPolicy2 string
	namePrefix := "tf-acc-test-"
	rInt := acctest.RandInt()
	resourceName := "minio_iam_group_policy.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckIAMGroupPolicyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMGroupPolicyConfigNamePrefix(namePrefix, rInt, "s3:ListAllMyBuckets"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIAMGroupPolicyExists(
						"minio_iam_group.test",
						"minio_iam_group_policy.test",
						&groupPolicy2,
					),
					resource.TestMatchResourceAttr(resourceName, "name", regexp.MustCompile(fmt.Sprintf("^%s", namePrefix))),
					resource.TestCheckResourceAttr(resourceName, "name_prefix", namePrefix),
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

func TestAccMinioIAMGroupPolicy_generatedName(t *testing.T) {
	var groupPolicy1 string
	rInt := acctest.RandInt()
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckIAMGroupPolicyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccIAMGroupPolicyConfigGeneratedName(rInt, "s3:ListBucket"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIAMGroupPolicyExists(
						"minio_iam_group.test",
						"minio_iam_group_policy.test",
						&groupPolicy1,
					),
					testAccCheckMinioIAMGroupPolicyNameExists(&groupPolicy1),
				),
			},
			{
				ResourceName:      "minio_iam_group_policy.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckIAMGroupPolicyDestroy(s *terraform.State) error {
	conn := testAccClient().S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_group_policy" {
			continue
		}

		_, name, err := resourceMinioIamGroupPolicyParseID(rs.Primary.ID)
		if err != nil {
			return err
		}

		if info, _ := conn.InfoCannedPolicy(context.Background(), name); info != nil {
			return fmt.Errorf("found IAM group policy, expected none %s: %s", name, err)

		}

	}

	return nil
}

func testAccCheckIAMGroupPolicyDisappears(
	iamGroupPolicyResource string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		iamconn := testAccClient().S3Admin

		policy, ok := s.RootModule().Resources[iamGroupPolicyResource]
		if !ok {
			return fmt.Errorf("not Found: %s", iamGroupPolicyResource)
		}

		_, name, err := resourceMinioIamGroupPolicyParseID(policy.Primary.ID)
		if err != nil {
			return err
		}

		if info, _ := iamconn.InfoCannedPolicy(context.Background(), name); info != nil {
			err = iamconn.RemoveCannedPolicy(context.Background(), name)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func testAccCheckIAMGroupPolicyExists(
	iamGroupResource string,
	iamGroupPolicyResource string,
	groupPolicy *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[iamGroupResource]
		if !ok {
			return fmt.Errorf("not Found: %s", iamGroupResource)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		policy, ok := s.RootModule().Resources[iamGroupPolicyResource]
		if !ok {
			return fmt.Errorf("not Found: %s", iamGroupPolicyResource)
		}

		_, name, err := resourceMinioIamGroupPolicyParseID(policy.Primary.ID)
		if err != nil {
			return err
		}

		*groupPolicy = name

		return nil
	}
}

func testAccCheckMinioIAMGroupPolicyNameChanged(i, j *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if *i == *j {
			return fmt.Errorf("the IAM Group Policy name did not change %s to %s", *i, *j)
		}

		return nil
	}
}

func testAccCheckMinioIAMGroupPolicyNameExists(i *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if *i == "" {
			return errors.New("the IAM Group Policy name does not exist")
		}

		return nil
	}
}

func testAccIAMGroupPolicyConfig(rInt int) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "group" {
  name = "test_group_%d"
}
resource "minio_iam_group_policy" "foo" {
  name  = "foo_policy_%d"
  group = "${minio_iam_group.group.group_name}"
  policy = <<EOF
{
	"Version": "2012-10-17",
	"Statement": [
		{
		"Effect": "Allow",
		"Action": "s3:ListAllMyBuckets",
		"Resource": ["arn:aws:s3:::*"]
	}
	]
}
EOF
}
`, rInt, rInt)
}

func testAccIAMGroupPolicyConfigNamePrefix(namePrefix string, rInt int, policyAction string) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "test" {
	name = "test_group_%d"
}
resource "minio_iam_group_policy" "test" {
  name_prefix = "%s"
  group       = "${minio_iam_group.test.group_name}"
  policy = <<EOF
{
	"Version": "2012-10-17",
	"Statement": [
		{
		"Effect": "Allow",
		"Action": "%s",
		"Resource": ["arn:aws:s3:::*"]
	}
	]
}
EOF
}
`, rInt, namePrefix, policyAction)
}

func testAccIAMGroupPolicyConfigGeneratedName(rInt int, policyAction string) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "test" {
  name = "test_group_%d"
}
resource "minio_iam_group_policy" "test" {
  group = "${minio_iam_group.test.group_name}"
  policy = <<EOF
{
	"Version": "2012-10-17",
	"Statement": [
		{
		"Effect": "Allow",
		"Action": "%s",
		"Resource": ["arn:aws:s3:::*"]
	}
	]
}
EOF
}
`, rInt, policyAction)
}

func testAccIAMGroupPolicyConfigUpdate(rInt int) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "group" {
	name = "test_group_%d"
}
resource "minio_iam_group_policy" "foo" {
  name   = "foo_policy_%d"
  group  = "${minio_iam_group.group.group_name}"
  policy = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":\"s3:ListAllMyBuckets\",\"Resource\":[\"arn:aws:s3:::*\"]}]}"
}
resource "minio_iam_group_policy" "bar" {
  name   = "bar_policy_%d"
  group  = "${minio_iam_group.group.group_name}"
  policy = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":\"s3:ListAllMyBuckets\",\"Resource\":[\"arn:aws:s3:::*\"]}]}"
}
`, rInt, rInt, rInt)
}

func testAccIAMGroupPolicyConfigWithAction(rInt int, action string) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "test" {
  name = "test_group_%d"
}

resource "minio_iam_group_policy" "test" {
  name  = "test_policy_%d"
  group = minio_iam_group.test.group_name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = "%s"
        Resource = ["arn:aws:s3:::*"]
      }
    ]
  })
}
`, rInt, rInt, action)
}

func testAccIAMGroupPolicyConfigWithMultipleActions(rInt int) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "test" {
  name = "test_group_%d"
}

resource "minio_iam_group_policy" "test" {
  name  = "test_policy_%d"
  group = minio_iam_group.test.group_name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
        Resource = ["arn:aws:s3:::*"]
      }
    ]
  })
}
`, rInt, rInt)
}

func TestAccMinioIAMGroupPolicy_longNames(t *testing.T) {
	rInt := acctest.RandInt()
	name100 := "tfacc-gp-" + strings.Repeat("a", 100-len("tfacc-gp-"))
	prefix230 := strings.Repeat("d", 230)
	var out string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckIAMGroupPolicyDestroy,
		Steps: []resource.TestStep{
			{
				Config:      testAccIAMGroupPolicyConfigNamePrefix(prefix230, rInt, "s3:ListAllMyBuckets"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)cannot\s+be\s+longer\s+than\s+229\s+characters`),
			},
			{
				Config: fmt.Sprintf(`
resource "minio_iam_group" "test" {
  name = "test_group_%d"
}
resource "minio_iam_group_policy" "test" {
  name   = %q
  group  = minio_iam_group.test.group_name
  policy = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":\"s3:ListAllMyBuckets\",\"Resource\":[\"arn:aws:s3:::*\"]}]}"
}
`, rInt, name100),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIAMGroupPolicyExists("minio_iam_group.test", "minio_iam_group_policy.test", &out),
					resource.TestCheckResourceAttr("minio_iam_group_policy.test", "name", name100),
				),
			},
		},
	})
}

func TestAccMinioIAMGroupPolicy_importNamePrefixNoReplacement(t *testing.T) {
	rInt := acctest.RandInt()
	groupName := fmt.Sprintf("test_group_%d", rInt)
	namePrefix := acctest.RandomWithPrefix("tf-acc-imp") + "-"
	policyName := id.PrefixedUniqueId(namePrefix)
	policyJSON := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:ListAllMyBuckets","Resource":["arn:aws:s3:::*"]}]}`
	resourceName := "minio_iam_group_policy.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckIAMGroupPolicyDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "minio_iam_group" "test" {
  name = %q
}
`, groupName),
			},
			{
				PreConfig: func() {
					if err := testAccClient().S3Admin.AddCannedPolicy(context.Background(), policyName, []byte(policyJSON)); err != nil {
						t.Fatalf("creating pre-existing policy %s: %v", policyName, err)
					}
				},
				Config:             testAccIAMGroupPolicyConfigNamePrefix(namePrefix, rInt, "s3:ListAllMyBuckets"),
				ResourceName:       resourceName,
				ImportState:        true,
				ImportStateId:      fmt.Sprintf("%s:%s", groupName, policyName),
				ImportStatePersist: true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					var gp *terraform.InstanceState
					for _, s := range states {
						if s.ID == fmt.Sprintf("%s:%s", groupName, policyName) {
							gp = s
							break
						}
					}
					if gp == nil {
						return fmt.Errorf("imported group policy state %s:%s not found", groupName, policyName)
					}
					if got := gp.Attributes["name_prefix"]; got != namePrefix {
						return fmt.Errorf("imported name_prefix = %q, want %q", got, namePrefix)
					}
					return nil
				},
			},
			{
				Config:             testAccIAMGroupPolicyConfigNamePrefix(namePrefix, rInt, "s3:ListAllMyBuckets"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}
