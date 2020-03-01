package minio

import (
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform/helper/acctest"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
)

func TestAccMinioIAMGroupPolicy_basic(t *testing.T) {
	var groupPolicy1, groupPolicy2 string
	rInt := acctest.RandInt()
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckIAMGroupPolicyDestroy,
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
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckIAMGroupPolicyDestroy,
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

func TestAccMinioIAMGroupPolicy_namePrefix(t *testing.T) {
	var groupPolicy2 string
	namePrefix := "tf-acc-test-"
	rInt := acctest.RandInt()
	resourceName := "minio_iam_group_policy.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckIAMGroupPolicyDestroy,
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
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"name_prefix"},
			},
		},
	})
}

func TestAccMinioIAMGroupPolicy_generatedName(t *testing.T) {
	var groupPolicy1 string
	rInt := acctest.RandInt()
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckIAMGroupPolicyDestroy,
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
	conn := testAccProvider.Meta().(*S3MinioClient).S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_group_policy" {
			continue
		}

		_, name, err := resourceMinioIamGroupPolicyParseID(rs.Primary.ID)
		if err != nil {
			return err
		}

		if output, _ := conn.InfoCannedPolicy(name); output != nil {
			return fmt.Errorf("Found IAM group policy, expected none %s: %s", name, err)

		}

	}

	return nil
}

func testAccCheckIAMGroupPolicyDisappears(
	iamGroupPolicyResource string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		iamconn := testAccProvider.Meta().(*S3MinioClient).S3Admin

		policy, ok := s.RootModule().Resources[iamGroupPolicyResource]
		if !ok {
			return fmt.Errorf("Not Found: %s", iamGroupPolicyResource)
		}

		_, name, err := resourceMinioIamGroupPolicyParseID(policy.Primary.ID)
		if err != nil {
			return err
		}

		if output, _ := iamconn.InfoCannedPolicy(name); output != nil {
			err = iamconn.RemoveCannedPolicy(name)
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
			return fmt.Errorf("Not Found: %s", iamGroupResource)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set")
		}

		policy, ok := s.RootModule().Resources[iamGroupPolicyResource]
		if !ok {
			return fmt.Errorf("Not Found: %s", iamGroupPolicyResource)
		}

		_, name, err := resourceMinioIamGroupPolicyParseID(policy.Primary.ID)
		if err != nil {
			return err
		}

		if err != nil {
			return err
		}

		*groupPolicy = name

		return nil
	}
}

func testAccCheckMinioIAMGroupPolicyNameChanged(i, j *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if aws.StringValue(i) == aws.StringValue(j) {
			return fmt.Errorf("IAM Group Policy name did not change %s to %s", *i, *j)
		}

		return nil
	}
}

func testAccCheckMinioIAMGroupPolicyNameExists(i *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if aws.StringValue(i) == "" {
			return errors.New("IAM Group Policy name does not exist")
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
