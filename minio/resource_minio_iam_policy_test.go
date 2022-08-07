package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioIAMPolicy_basic(t *testing.T) {
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
					testAccCheckMinioIAMPolicyExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", rName),
					testCheckJSONResourceAttr(resourceName, "policy", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":["arn:aws:s3:::*"]}]}`),
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
					testAccCheckMinioIAMPolicyExists(resourceName),
					testAccCheckMinioIAMPolicyDisappears(rName),
				),
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestAccMinioIAMPolicy_namePrefix(t *testing.T) {
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
					testAccCheckMinioIAMPolicyExists(resourceName),
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
					testAccCheckMinioIAMPolicyExists(resourceName),
					testCheckJSONResourceAttr(resourceName, "policy", policy1),
				),
			},
			{
				Config: testAccMinioIAMPolicyConfigPolicy(rName2, policy2),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMPolicyExists(resourceName),
					testCheckJSONResourceAttr(resourceName, "policy", policy2),
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

func testAccCheckMinioIAMPolicyExists(resource string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resource]
		if !ok {
			return fmt.Errorf("not found: %s", resource)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no Policy name is set")
		}

		iamconn := testAccProvider.Meta().(*S3MinioClient).S3Admin

		_, err := iamconn.InfoCannedPolicy(context.Background(), rs.Primary.ID)
		return err
	}
}

func testAccCheckMinioIAMPolicyDestroy(s *terraform.State) error {
	iamconn := testAccProvider.Meta().(*S3MinioClient).S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_policy" {
			continue
		}

		if output, _ := iamconn.InfoCannedPolicy(context.Background(), rs.Primary.ID); output != nil {
			return fmt.Errorf("iAM Policy (%s) still exists", rs.Primary.ID)
		}

	}

	return nil
}

func testAccCheckMinioIAMPolicyDisappears(resource string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		iamconn := testAccProvider.Meta().(*S3MinioClient).S3Admin

		if output, _ := iamconn.InfoCannedPolicy(context.Background(), resource); output == nil {

			if err := iamconn.RemoveCannedPolicy(context.Background(), resource); err != nil {
				return err
			}

		}
		return nil
	}
}

func testCheckJSONResourceAttr(name, key, value string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("not found: %s", name)
		}

		v, ok := rs.Primary.Attributes[key]
		if !ok {
			return fmt.Errorf("%s: Attribute '%s' not found", name, key)
		}

		var actual, expected interface{}
		_ = json.Unmarshal([]byte(value), &expected)
		_ = json.Unmarshal([]byte(v), &actual)
		diff := cmp.Diff(actual, expected)
		if diff != "" {
			return fmt.Errorf("%s: mismatch (-want +got):\n%s", name, diff)
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
