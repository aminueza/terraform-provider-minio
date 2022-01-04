package minio

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	iampolicy "github.com/minio/minio/pkg/iam/policy"
)

func TestAccMinioIAMPolicy_basic(t *testing.T) {
	var out iampolicy.Policy
	rName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_iam_policy.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMPolicyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMPolicyConfigName(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMPolicyExists(resourceName, &out),
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					resource.TestCheckResourceAttr(resourceName, "policy", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":["arn:aws:s3:::*"]}]}`),
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
func TestAccMinioIAMPolicy_disappears(t *testing.T) {
	var out iampolicy.Policy
	rName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_iam_policy.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMPolicyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMPolicyConfigName(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMPolicyExists(resourceName, &out),
					testAccCheckMinioIAMPolicyDisappears(rName),
				),
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestAccMinioIAMPolicy_namePrefix(t *testing.T) {
	var out iampolicy.Policy
	namePrefix := "tf-acc-test-"
	resourceName := "minio_iam_policy.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMPolicyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioIAMPolicyConfigNamePrefix(namePrefix),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMPolicyExists(resourceName, &out),
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

func TestAccMinioIAMPolicy_policy(t *testing.T) {
	var out iampolicy.Policy
	rName1 := acctest.RandomWithPrefix("tf-acc-test")
	rName2 := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_iam_policy.test"
	policy1 := "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":[\"s3:ListBucket\"],\"Resource\":[\"arn:aws:s3:::*\"]}]}"
	policy2 := "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":[\"s3:*\"],\"Resource\":[\"arn:aws:s3:::*\"]}]}"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMPolicyDestroy,
		Steps: []resource.TestStep{
			{
				Config:      testAccMinioIAMPolicyConfigPolicy(rName1, "not-json"),
				ExpectError: regexp.MustCompile("invalid JSON"),
			},
			{
				Config: testAccMinioIAMPolicyConfigPolicy(rName1, policy1),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMPolicyExists(resourceName, &out),
					resource.TestCheckResourceAttr(resourceName, "policy", policy1),
				),
			},
			{
				Config: testAccMinioIAMPolicyConfigPolicy(rName2, policy2),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMPolicyExists(resourceName, &out),
					resource.TestCheckResourceAttr(resourceName, "policy", policy2),
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

func testAccCheckMinioIAMPolicyExists(resource string, res *iampolicy.Policy) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resource]
		if !ok {
			return fmt.Errorf("Not found: %s", resource)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No Policy name is set")
		}

		iamconn := testAccProvider.Meta().(*S3MinioClient).S3Admin

		if resp, err := iamconn.InfoCannedPolicy(context.Background(), rs.Primary.ID); res != nil {
			if err != nil {
				return err
			}

			res = resp
		}

		return nil
	}
}

func testAccCheckMinioIAMPolicyDestroy(s *terraform.State) error {
	iamconn := testAccProvider.Meta().(*S3MinioClient).S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_policy" {
			continue
		}

		if output, _ := iamconn.InfoCannedPolicy(context.Background(), rs.Primary.ID); output != nil {
			return fmt.Errorf("IAM Policy (%s) still exists", rs.Primary.ID)
		}

	}

	return nil
}

func testAccCheckMinioIAMPolicyDisappears(out string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		iamconn := testAccProvider.Meta().(*S3MinioClient).S3Admin

		if output, _ := iamconn.InfoCannedPolicy(context.Background(), out); output == nil {

			if err := iamconn.RemoveCannedPolicy(context.Background(), out); err != nil {
				return err
			}

		}
		return nil
	}
}

func testAccMinioIAMPolicyConfigDescription(rName, description string) string {
	return fmt.Sprintf(`
resource "minio_iam_policy" "test" {
  description = %q
  name        = %q
  policy      = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Action\":[\"s3:ListBucket*\"],\"Effect\":\"Allow\",\"Resource\":[\"arn:aws:s3:::*\"]}]}"
}
`, description, rName)
}

func testAccMinioIAMPolicyConfigName(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_policy" "test" {
  name   = %q
  policy = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Action\":[\"s3:ListBucket\"],\"Effect\":\"Allow\",\"Resource\":[\"arn:aws:s3:::*\"]}]}"
}
`, rName)
}

func testAccMinioIAMPolicyConfigNamePrefix(namePrefix string) string {
	return fmt.Sprintf(`
resource "minio_iam_policy" "test" {
  name_prefix = %q
  policy      = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Action\":[\"s3:ListBucket\"],\"Effect\":\"Allow\",\"Resource\":[\"arn:aws:s3:::*\"]}]}"
}
`, namePrefix)
}

func testAccMinioIAMPolicyConfigPolicy(rName, policy string) string {
	return fmt.Sprintf(`
resource "minio_iam_policy" "test" {
  name   = %q
  policy = %q
}
`, rName, policy)
}
