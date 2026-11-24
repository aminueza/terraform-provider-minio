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
	"github.com/minio/madmin-go/v4"
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

		if _, err := conn.InfoCannedPolicy(context.Background(), name); err == nil {
			return fmt.Errorf("found IAM group policy, expected none: %s", name)
		} else if !isNoSuchPolicyError(err) {
			return fmt.Errorf("checking IAM group policy %s destroy: %w", name, err)
		}
	}

	return nil
}

func testAccCheckIAMGroupPolicyDisappears(
	iamGroupPolicyResource string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		iamconn := testAccClient().S3Admin

		policy, ok := s.RootModule().Resources[iamGroupPolicyResource]
		if !ok {
			return fmt.Errorf("not Found: %s", iamGroupPolicyResource)
		}

		groupName, name, err := resourceMinioIamGroupPolicyParseID(policy.Primary.ID)
		if err != nil {
			return err
		}

		if _, err := iamconn.DetachPolicy(ctx, madmin.PolicyAssociationReq{
			Policies: []string{name},
			Group:    groupName,
		}); err != nil && !isNoSuchGroupError(err) {
			return err
		}

		if info, _ := iamconn.InfoCannedPolicy(ctx, name); info != nil {
			err = iamconn.RemoveCannedPolicy(ctx, name)
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
		ctx := context.Background()
		conn := testAccClient().S3Admin

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

		groupName, name, err := resourceMinioIamGroupPolicyParseID(policy.Primary.ID)
		if err != nil {
			return err
		}

		if _, err := conn.InfoCannedPolicy(ctx, name); err != nil {
			return fmt.Errorf("reading IAM group policy %s: %w", name, err)
		}

		policies, err := minioReadGroupPolicies(ctx, conn, groupName)
		if err != nil {
			return fmt.Errorf("reading IAM group %s: %w", groupName, err)
		}
		if !Contains(policies, name) {
			return fmt.Errorf("IAM policy %q is not attached to group %q (attached: %v)", name, groupName, policies)
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
					ctx := context.Background()
					admin := testAccClient().S3Admin
					if err := admin.AddCannedPolicy(ctx, policyName, []byte(policyJSON)); err != nil {
						t.Fatalf("creating pre-existing policy %s: %v", policyName, err)
					}
					if _, err := admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{
						Policies: []string{policyName},
						Group:    groupName,
					}); err != nil {
						t.Fatalf("attaching pre-existing policy %s to group %s: %v", policyName, groupName, err)
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

func TestAccMinioIAMGroupPolicy_attachDetachAndValidation(t *testing.T) {
	rInt := acctest.RandInt()
	groupName := fmt.Sprintf("developer-%d", rInt)
	groupPolicyName := fmt.Sprintf("state-terraform-s3-%d", rInt)
	attachedPolicyName := fmt.Sprintf("attached-pol-%d", rInt)
	var recorded string

	baseWithBoth := fmt.Sprintf(`
resource "minio_iam_group" "developer" {
  name = %[1]q
}

resource "minio_iam_policy" "extra" {
  name   = %[2]q
  policy = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":[\"s3:GetObject\"],\"Resource\":[\"arn:aws:s3:::*\"]}]}"
}

resource "minio_iam_group_policy_attachment" "extra" {
  group_name  = minio_iam_group.developer.name
  policy_name = minio_iam_policy.extra.id
}

resource "minio_iam_group_policy" "test_policy" {
  name  = %[3]q
  group = minio_iam_group.developer.id
  policy = <<EOF
{
  "Version":"2012-10-17",
  "Statement": [
    {
      "Sid":"ListAllBucket",
      "Effect": "Allow",
      "Action": ["s3:PutObject"],
      "Principal":"*",
      "Resource": "arn:aws:s3:::%[3]s/*"
    }
  ]
}
EOF
  depends_on = [minio_iam_group_policy_attachment.extra]
}
`, groupName, attachedPolicyName, groupPolicyName)

	withoutGroupPolicy := fmt.Sprintf(`
resource "minio_iam_group" "developer" {
  name = %[1]q
}

resource "minio_iam_policy" "extra" {
  name   = %[2]q
  policy = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":[\"s3:GetObject\"],\"Resource\":[\"arn:aws:s3:::*\"]}]}"
}

resource "minio_iam_group_policy_attachment" "extra" {
  group_name  = minio_iam_group.developer.name
  policy_name = minio_iam_policy.extra.id
}
`, groupName, attachedPolicyName)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckIAMGroupPolicyDestroy,
		Steps: []resource.TestStep{
			{
				Config: baseWithBoth,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIAMGroupPolicyExists("minio_iam_group.developer", "minio_iam_group_policy.test_policy", &recorded),
					func(s *terraform.State) error {
						policies, err := minioReadGroupPolicies(context.Background(), testAccClient().S3Admin, groupName)
						if err != nil {
							return err
						}
						if !Contains(policies, attachedPolicyName) || !Contains(policies, groupPolicyName) {
							return fmt.Errorf("expected both %q and %q attached to %q, got %v", attachedPolicyName, groupPolicyName, groupName, policies)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "minio_iam_group_policy.test_policy",
				ImportState:       true,
				ImportStateId:     fmt.Sprintf("%s:%s", groupName, groupPolicyName),
				ImportStateVerify: true,
			},
			{
				ResourceName:  "minio_iam_group_policy.test_policy",
				ImportState:   true,
				ImportStateId: "developer",
				ExpectError:   regexp.MustCompile(`unexpected format of ID \("developer"\), expected <group-name>:<policy-name>`),
			},
			{
				Config: withoutGroupPolicy,
				Check: resource.ComposeTestCheckFunc(
					func(s *terraform.State) error {
						ctx := context.Background()
						conn := testAccClient().S3Admin
						policies, err := minioReadGroupPolicies(ctx, conn, groupName)
						if err != nil {
							return err
						}
						if Contains(policies, groupPolicyName) {
							return fmt.Errorf("expected %q to be detached from %q, still got %v", groupPolicyName, groupName, policies)
						}
						if !Contains(policies, attachedPolicyName) {
							return fmt.Errorf("expected attachment %q to stay attached to %q, got %v", attachedPolicyName, groupName, policies)
						}
						if _, err := conn.InfoCannedPolicy(ctx, groupPolicyName); !isNoSuchPolicyError(err) {
							return fmt.Errorf("expected canned policy %q to be deleted, InfoCannedPolicy err = %v", groupPolicyName, err)
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccMinioIAMGroupPolicy_externalDetachPlansCreate(t *testing.T) {
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
					testAccCheckIAMGroupPolicyExists("minio_iam_group.group", "minio_iam_group_policy.foo", &out),
					func(s *terraform.State) error {
						policy, ok := s.RootModule().Resources["minio_iam_group_policy.foo"]
						if !ok {
							return fmt.Errorf("not Found: minio_iam_group_policy.foo")
						}
						groupName, name, err := resourceMinioIamGroupPolicyParseID(policy.Primary.ID)
						if err != nil {
							return err
						}
						// The refresh after the detach drops the resource from
						// state, so the SDK destroy would skip the canned
						// policy; remove it here so nothing is left behind.
						t.Cleanup(func() {
							_ = testAccClient().S3Admin.RemoveCannedPolicy(context.Background(), name)
						})
						_, err = testAccClient().S3Admin.DetachPolicy(context.Background(), madmin.PolicyAssociationReq{
							Policies: []string{name},
							Group:    groupName,
						})
						return err
					},
				),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccMinioIAMGroupPolicy_invalidGroupErrors(t *testing.T) {
	missingGroup := fmt.Sprintf("does-not-exist-%d", acctest.RandInt())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckIAMGroupPolicyDestroy,
		Steps: []resource.TestStep{
			{
				Config: `
resource "minio_iam_group_policy" "bad" {
  name   = "bad_colon_policy"
  group  = "a:b"
  policy = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":\"s3:ListAllMyBuckets\",\"Resource\":[\"arn:aws:s3:::*\"]}]}"
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`only alphanumeric characters`),
			},
			{
				Config: fmt.Sprintf(`
resource "minio_iam_group_policy" "missing" {
  name   = "missing_group_policy_%d"
  group  = %q
  policy = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":\"s3:ListAllMyBuckets\",\"Resource\":[\"arn:aws:s3:::*\"]}]}"
}
`, acctest.RandInt(), missingGroup),
				ExpectError: regexp.MustCompile(regexp.QuoteMeta(missingGroup)),
			},
		},
	})
}
