package minio

import (
	"fmt"
	"testing"

	"github.com/aminueza/terraform-minio-provider/madmin"
	"github.com/hashicorp/terraform/helper/acctest"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
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
	var conf madmin.UserInfo

	name1 := fmt.Sprintf("test-user-%d", acctest.RandInt())
	name2 := fmt.Sprintf("test-user-%d", acctest.RandInt())
	status1 := "enabled"
	status2 := "disabled"
	resourceName := "minio_iam_user.user"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioUserConfig(name1, status1),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists("minio_iam_user.user", &conf),
					testAccCheckMinioUserAttributes(&conf, name1, status1),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"force_destroy"},
			},
			{
				Config: testAccMinioUserConfig(name2, status2),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists("minio_iam_user.user", &conf),
					testAccCheckMinioUserAttributes(&conf, name2, status2),
				),
			},
		},
	})
}

func TestAccAWSUser_RotateAccessKey(t *testing.T) {
	var user madmin.UserInfo

	rName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_iam_user.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioUserConfigForceDestroy(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
					testAccCheckMinioUserRotatesAccessKey(&user),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"force_destroy"},
			},
			{
				Config: testAccMinioUserConfigUpdateSecret(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
					testAccCheckMinioUserRotatesAccessKey(&user),
				),
			},
		},
	})
}

func testAccMinioUserConfig(rName, rStatus string) string {
	return fmt.Sprintf(`
	resource "minio_iam_user" "user" {
		  name = %q
		  status = %q
		}`, rName, rStatus)
}

func testAccMinioUserConfigForceDestroy(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test" {
  force_destroy = true
  name          = %q
}
`, rName)
}

func testAccMinioUserConfigUpdateSecret(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test" {
  update_secret = true
  name          = %q
}
`, rName)
}

func testAccCheckMinioUserExists(n string, res *madmin.UserInfo) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No User name is set")
		}

		minioIam := testAccProvider.Meta().(*S3MinioClient).S3Admin

		resp, err := minioIam.GetUserInfo(rs.Primary.ID)
		if err != nil {
			return err
		}

		*res = resp

		return nil
	}
}

func testAccCheckMinioUserAttributes(user *madmin.UserInfo, name string, status string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		userMinio := user
		if userMinio.SecretKey != name {
			return fmt.Errorf("Bad name: %s", user.SecretKey)
		}

		if userMinio.Status != madmin.AccountStatus(status) {
			return fmt.Errorf("Bad status: %s", user.Status)
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
		_, err := minioIam.GetUserInfo(rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("still exist.")
		}

	}

	return nil
}

func testAccCheckMinioUserRotatesAccessKey(user *madmin.UserInfo) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		minioIam := testAccProvider.Meta().(*S3MinioClient).S3Admin

		secretKey, _ := generateSecretAccessKey()

		userStatus := UserStatus{
			AccessKey: user.SecretKey,
			SecretKey: string(secretKey),
			Status:    madmin.AccountStatus(statusUser(false)),
		}

		if err := minioIam.SetUser(userStatus.AccessKey, userStatus.SecretKey, userStatus.Status); err != nil {
			return fmt.Errorf("error rotating IAM User (%s) Access Key: %s", userStatus.AccessKey, err)
		}

		return nil
	}
}
