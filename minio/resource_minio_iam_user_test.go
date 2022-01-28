package minio

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go"
)

func TestValidateMinioIamUserName(t *testing.T) {
	minioValidNames := []string{
		"test-user",
		"test_user",
		"testuser123",
		"TestUser",
		"Test-User",
		"test.user",
		"test.123,user",
		"testuser@minio",
		"test+user@minio.io",
	}

	for _, minioName := range minioValidNames {
		_, err := validateMinioIamUserName(minioName, "name")
		if len(err) != 0 {
			t.Fatalf("%q should be a valid IAM User name: %q", minioName, err)
		}
	}

	minioInvalidNames := []string{
		"!",
		"/",
		" ",
		":",
		";",
		"test name",
		"/slash-at-the-beginning",
		"slash-at-the-end/",
	}

	for _, minioName := range minioInvalidNames {
		_, err := validateMinioIamUserName(minioName, "name")
		if len(err) == 0 {
			t.Fatalf("%q should be an invalid IAM User name", minioName)
		}
	}
}

func TestAccAWSUser_basic(t *testing.T) {
	var user madmin.UserInfo

	name := fmt.Sprintf("test-user-%d", acctest.RandInt())
	status := "enabled"
	resourceName := "minio_iam_user.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioUserConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
					testAccCheckMinioUserAttributes(resourceName, name, status),
				),
			},
		},
	})
}

func TestAccAWSUser_DisableUser(t *testing.T) {
	var user madmin.UserInfo

	name := fmt.Sprintf("test-user-%d", acctest.RandInt())
	resourceName := "minio_iam_user.test1"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioUserConfigDisabled(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
					testAccCheckMinioUserDisabled(resourceName),
				),
			},
		},
	})
}

func TestAccAWSUser_RotateAccessKey(t *testing.T) {
	var user madmin.UserInfo

	name := fmt.Sprintf("test-user-%d", acctest.RandInt())
	resourceName := "minio_iam_user.test3"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioUserConfigUpdateSecret(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
					testAccCheckMinioUserRotatesAccessKey(resourceName),
				),
			},
		},
	})
}

func testAccMinioUserConfig(rName string) string {
	return fmt.Sprintf(`
	resource "minio_iam_user" "test" {
		  name = %q
		}`, rName)
}

func testAccMinioUserConfigDisabled(rName string) string {
	return fmt.Sprintf(`
	resource "minio_iam_user" "test1" {
		  name         = %q
		  disable_user = true
		}`, rName)
}

func testAccMinioUserConfigForceDestroy(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test2" {
  force_destroy = true
  name          = %q
}
`, rName)
}

func testAccMinioUserConfigUpdateSecret(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test3" {
  update_secret = true
  name          = %q
}
`, rName)
}

func testAccCheckMinioUserExists(n string, res *madmin.UserInfo) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s %s", n, s)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No User name is set")
		}

		minioIam := testAccProvider.Meta().(*S3MinioClient).S3Admin

		resp, err := minioIam.GetUserInfo(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}

		res = &resp

		return nil
	}
}

func testAccCheckMinioUserDisabled(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s %s", n, s)
		}

		minioIam := testAccProvider.Meta().(*S3MinioClient).S3Admin

		resp, err := minioIam.GetUserInfo(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("Error getting user %s", err)
		}

		if rs.Primary.Attributes["status"] != string(madmin.AccountDisabled) || resp.Status != madmin.AccountDisabled {
			return fmt.Errorf("User still enabled: state:%s server:%s", rs.Primary.Attributes["status"], resp.Status)
		}

		return nil
	}
}

func testAccCheckMinioUserAttributes(n string, name string, status string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, _ := s.RootModule().Resources[n]

		if rs.Primary.Attributes["name"] != name {
			return fmt.Errorf("Bad name: %s", name)
		}

		if rs.Primary.Attributes["status"] != status {
			return fmt.Errorf("Bad status: %s", status)
		}

		return nil
	}
}

func testAccCheckMinioUserDestroy(s *terraform.State) error {
	minioIam := testAccProvider.Meta().(*S3MinioClient).S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_user" {
			continue
		}

		// Try to get user
		_, err := minioIam.GetUserInfo(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("User still exists")
		}

	}

	return nil
}

func testAccCheckMinioUserRotatesAccessKey(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, _ := s.RootModule().Resources[n]

		minioIam := testAccProvider.Meta().(*S3MinioClient).S3Admin

		secretKey, _ := generateSecretAccessKey()

		userStatus := UserStatus{
			AccessKey: rs.Primary.ID,
			SecretKey: string(secretKey),
			Status:    madmin.AccountDisabled,
		}

		if err := minioIam.SetUser(context.Background(), userStatus.AccessKey, userStatus.SecretKey, userStatus.Status); err != nil {
			return fmt.Errorf("error rotating IAM User (%s) Access Key: %s", userStatus.AccessKey, err)
		}

		return nil
	}
}
