package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	awspolicy "github.com/hashicorp/awspolicyequivalence"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioIAMPolicy_basic(t *testing.T) {
	rName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_iam_policy.test"
	expectedPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":["arn:aws:s3:::*"]}]}`

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
					testCheckJSONResourceAttr(resourceName, "policy", expectedPolicy),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"policy"},
				Check: resource.ComposeTestCheckFunc(
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources[resourceName]
						if !ok {
							return fmt.Errorf("Not found: %s", resourceName)
						}

						importedPolicy := rs.Primary.Attributes["policy"]
						equivalent, err := awspolicy.PoliciesAreEquivalent(expectedPolicy, importedPolicy)
						if err != nil {
							return fmt.Errorf("Error comparing policies: %s", err)
						}
						if !equivalent {
							return fmt.Errorf("Imported policy is not equivalent to expected policy")
						}
						return nil
					},
				),
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

func TestAccMinioIAMPolicy_recreate(t *testing.T) {
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
				),
				ExpectNonEmptyPlan: false,
			},
			{
				PreConfig: func() {
					_ = testAccCheckMinioIAMPolicyDeleteExternally(rName)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccMinioIAMPolicyConfigName(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMPolicyExists(resourceName),
				),
			},
		},
	})
}

func TestAccMinioIAMPolicy_namePrefix(t *testing.T) {
	namePrefix := "tf-acc-test-"
	resourceName := "minio_iam_policy.test"
	expectedPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":["arn:aws:s3:::*"]}]}`

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
				ImportStateVerifyIgnore: []string{"name_prefix", "policy"},
				Check: resource.ComposeTestCheckFunc(
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources[resourceName]
						if !ok {
							return fmt.Errorf("Not found: %s", resourceName)
						}

						importedPolicy := rs.Primary.Attributes["policy"]
						equivalent, err := awspolicy.PoliciesAreEquivalent(expectedPolicy, importedPolicy)
						if err != nil {
							return fmt.Errorf("Error comparing policies: %s", err)
						}
						if !equivalent {
							return fmt.Errorf("Imported policy is not equivalent to expected policy")
						}
						return nil
					},
				),
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
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"policy"},
				Check: resource.ComposeTestCheckFunc(
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources[resourceName]
						if !ok {
							return fmt.Errorf("Not found: %s", resourceName)
						}

						importedPolicy := rs.Primary.Attributes["policy"]
						equivalent, err := awspolicy.PoliciesAreEquivalent(policy2, importedPolicy)
						if err != nil {
							return fmt.Errorf("Error comparing policies: %s", err)
						}
						if !equivalent {
							return fmt.Errorf("Imported policy is not equivalent to expected policy")
						}
						return nil
					},
				),
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

func testAccCheckMinioIAMPolicyDeleteExternally(rName string) error {
	minioIam := testAccProvider.Meta().(*S3MinioClient).S3Admin

	if err := minioIam.RemoveCannedPolicy(context.Background(), rName); err != nil {
		return fmt.Errorf("policy could not be deleted: %w", err)
	}

	return nil
}

// TestAccMinioIAMPolicy_jsonencode tests that policies with jsonencode don't trigger
// unnecessary updates, which was the issue reported in GitHub issue #348
func TestAccMinioIAMPolicy_jsonencode(t *testing.T) {
	resourceName := "minio_iam_policy.test_jsonencode"
	rName := acctest.RandomWithPrefix("tf-acc-jsonencode")

	// Run the test in multiple phases to verify the policy remains unchanged
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioIAMPolicyDestroy,
		Steps: []resource.TestStep{
			// Apply the policy for the first time
			{
				Config: testAccMinioIAMPolicyConfigJsonencode(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMPolicyExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", rName),
				),
				ExpectNonEmptyPlan: false,
			},
			// Apply the exact same policy a second time - should produce no changes
			{
				Config: testAccMinioIAMPolicyConfigJsonencode(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMPolicyExists(resourceName),
				),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			// Change the order of the actions, which should still be semantically equivalent
			{
				Config: testAccMinioIAMPolicyConfigJsonencodeReordered(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMPolicyExists(resourceName),
				),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			// Verify import works correctly
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"policy"}, // Skip policy verification during import
			},
			// Use a final step with a specific check for semantically equivalent policies
			{
				Config: testAccMinioIAMPolicyConfigJsonencode(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioIAMPolicyExists(resourceName),
					// Use the testCheckJSONResourceAttr helper that properly compares JSON content
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources[resourceName]
						if !ok {
							return fmt.Errorf("resource not found: %s", resourceName)
						}

						policyText := rs.Primary.Attributes["policy"]
						// Verify the policy is semantically equivalent to what we expect
						equivalent, err := awspolicy.PoliciesAreEquivalent(policyText, getExpectedJsonPolicy())
						if err != nil {
							return fmt.Errorf("error comparing policies: %s", err)
						}
						if !equivalent {
							return fmt.Errorf("policies are not equivalent")
						}
						return nil
					},
				),
			},
		},
	})
}

// testAccMinioIAMPolicyConfigJsonencode creates a policy using jsonencode just like in the GitHub issue
func testAccMinioIAMPolicyConfigJsonencode(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_policy" "test_jsonencode" {
  name = %q
  policy = <<-EOT
  {
      "Version": "2012-10-17",
      "Statement": ${jsonencode([
        {
          "Effect": "Allow",
          "Action": [
            "s3:ListBucket", 
            "s3:GetObject",
            "s3:PutObject"
          ],
          "Resource": [
            "arn:aws:s3:::*"
          ]
        }
      ])}
  }
EOT
}
`, rName)
}

// testAccMinioIAMPolicyConfigJsonencodeReordered has the same content but with actions in different order
func testAccMinioIAMPolicyConfigJsonencodeReordered(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_policy" "test_jsonencode" {
  name = %q
  policy = <<-EOT
  {
      "Version": "2012-10-17",
      "Statement": ${jsonencode([
        {
          "Effect": "Allow",
          "Action": [
            "s3:GetObject",
            "s3:ListBucket",
            "s3:PutObject"
          ],
          "Resource": [
            "arn:aws:s3:::*"
          ]
        }
      ])}
  }
EOT
}
`, rName)
}

// getExpectedJsonPolicy returns a normalized JSON policy string for comparison
func getExpectedJsonPolicy() string {
	return `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket","s3:GetObject","s3:PutObject"],"Resource":["arn:aws:s3:::*"]}]}`
}
