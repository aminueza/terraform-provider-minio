package minio

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v4"
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
		"CN=Backup Operators,CN=Builtin,DC=gr-u,DC=it",
		"cn=Backup Operators,cn=Builtin,dc=gr-u,dc=it",
		"CN=View-Only Organization Management,OU=Microsoft Exchange Security Groups,DC=gr-u,DC=it",
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
		"OU=Microsoft Exchange Security Groups,DC=gr-u,DC=it",
		"OU=Microsoft Exchange Security Groups",
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
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"secret", "update_secret", "force_destroy"},
			},
		},
	})
}

func TestAccAWSUser_UpdateName(t *testing.T) {
	var user madmin.UserInfo

	name := fmt.Sprintf("test-user-%d", acctest.RandInt())
	status := "enabled"
	resourceName := "minio_iam_user.test"
	updatedName := fmt.Sprintf("test-user-%d", acctest.RandInt())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioUserConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
				),
			},
			{
				Config: testAccMinioUserConfig(updatedName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserAttributes(resourceName, updatedName, status),
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

func TestAccMinioIAMUser_statusChangedExternally(t *testing.T) {
	name := fmt.Sprintf("test-user-%d", acctest.RandInt())
	resourceName := "minio_iam_user.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioUserConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserStatus(resourceName, "false", "enabled"),
				),
			},
			{
				PreConfig: func() {
					if err := testAccCheckMinioUserSetStatusExternally(name, madmin.AccountDisabled); err != nil {
						t.Fatal(err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserStatus(resourceName, "true", "disabled"),
				),
			},
			{
				Config: testAccMinioUserConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserStatus(resourceName, "false", "enabled"),
				),
			},
			{
				Config: testAccMinioUserConfigWithDisable(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserStatus(resourceName, "true", "disabled"),
				),
			},
			{
				PreConfig: func() {
					if err := testAccCheckMinioUserSetStatusExternally(name, madmin.AccountEnabled); err != nil {
						t.Fatal(err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserStatus(resourceName, "false", "enabled"),
				),
			},
			{
				Config: testAccMinioUserConfigWithDisable(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserStatus(resourceName, "true", "disabled"),
				),
			},
		},
	})
}

func TestAccAWSUser_RotateAccessKey(t *testing.T) {
	var user madmin.UserInfo
	var oldAccessKey string

	name := fmt.Sprintf("test-user-%d", acctest.RandInt())
	resourceName := "minio_iam_user.test3"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioUserConfigWithoutSecret(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
					testAccCheckMinioUserExfiltrateAccessKey(resourceName, &oldAccessKey),
					testAccCheckMinioUserCanLogIn(resourceName),
				),
			},
			{
				Config: testAccMinioUserConfigUpdateSecret(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
					testAccCheckMinioUserRotatesAccessKey(resourceName, &oldAccessKey),
					testAccCheckMinioUserCanLogIn(resourceName),
				),
			},
		},
	})
}

func TestAccAWSUser_SettingAccessKey(t *testing.T) {
	var user madmin.UserInfo

	name := fmt.Sprintf("test-user-%d", acctest.RandInt())
	resourceName := "minio_iam_user.test4"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioUserConfigSetSecret(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
					testAccCheckMinioUserCanLogIn(resourceName),
				),
			},
		},
	})
}

func TestAccAWSUser_UpdateAccessKey(t *testing.T) {
	var user madmin.UserInfo
	var oldAccessKey string

	name := fmt.Sprintf("test-user-%d", acctest.RandInt())
	resourceName := "minio_iam_user.test5"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioUserConfigWithSecretOne(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
					testAccCheckMinioUserExfiltrateAccessKey(resourceName, &oldAccessKey),
					testAccCheckMinioUserCanLogIn(resourceName),
				),
			},
			{
				Config: testAccMinioUserConfigWithSecretTwo(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
					testAccCheckMinioUserRotatesAccessKey(resourceName, &oldAccessKey),
					testAccCheckMinioUserCanLogIn(resourceName),
				),
			},
		},
	})
}

func TestAccAWSUser_RecreateMissing(t *testing.T) {
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
			{
				PreConfig: func() {
					_ = testAccCheckMinioUserDeleteExternally(name)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
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

func TestAccAWSUser_WriteOnlySecret_basic(t *testing.T) {
	var user madmin.UserInfo

	name := fmt.Sprintf("test-user-wo-%d", acctest.RandInt())
	resourceName := "minio_iam_user.testwo"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioUserConfigSetSecretWO(name, "secret1234", 1),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
					resource.TestCheckResourceAttr(resourceName, "secret", ""),
					resource.TestCheckNoResourceAttr(resourceName, "secret_wo"),
				),
			},
		},
	})
}

func TestAccAWSUser_WriteOnlySecret_transition(t *testing.T) {
	var user madmin.UserInfo

	name := fmt.Sprintf("test-user-wo-transition-%d", acctest.RandInt())
	resourceName := "minio_iam_user.testwo_transition"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioUserConfigSensitiveTransitionOne(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
					resource.TestCheckResourceAttr(resourceName, "secret", "secret1234"),
				),
			},
			{
				Config: testAccMinioUserConfigSensitiveToWriteOnly(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
					resource.TestCheckResourceAttr(resourceName, "secret", ""),
				),
			},
			{
				Config: testAccMinioUserConfigSensitiveTransitionTwo(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioUserExists(resourceName, &user),
					resource.TestCheckResourceAttr(resourceName, "secret", "secret4321"),
				),
			},
		},
	})
}

func testAccMinioUserConfigWithSecretOne(rName string) string {
	return fmt.Sprintf(`
	resource "minio_iam_user" "test5" {
	  secret = "secret1234"
	  name   = %q
	}
	`, rName)
}

func testAccMinioUserConfigSetSecretWO(rName string, secret string, version int) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "testwo" {
  name              = %q
  secret_wo         = %q
  secret_wo_version = %d
}
`, rName, secret, version)
}

func testAccMinioUserConfigSensitiveTransitionOne(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "testwo_transition" {
  name   = %q
  secret = "secret1234"
}
`, rName)
}

func testAccMinioUserConfigSensitiveToWriteOnly(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "testwo_transition" {
  name              = %q
  secret_wo         = "secret5678"
  secret_wo_version = 1
}
`, rName)
}

func testAccMinioUserConfigSensitiveTransitionTwo(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "testwo_transition" {
  name   = %q
  secret = "secret4321"
}
`, rName)
}
func testAccMinioUserConfigWithSecretTwo(rName string) string {
	return fmt.Sprintf(`
	resource "minio_iam_user" "test5" {
	  secret = "secret4321"
	  name   = %q
	}
	`, rName)
}

func TestAccMinioIAMUser_createFailsWhenExists(t *testing.T) {
	name := fmt.Sprintf("test-user-%s", acctest.RandString(8))

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { testAccCreateUserOutsideTerraform(t, name) },
				Config:      testAccMinioUserConfig(name),
				ExpectError: regexp.MustCompile(`already exists`),
			},
		},
	})
}

// testAccCreateUserOutsideTerraform seeds a user that no Terraform state knows
// about and removes it again when the test ends, since CheckDestroy only walks
// resources in state.
func testAccCreateUserOutsideTerraform(t *testing.T, name string) {
	ctx := context.Background()
	minioIam := testAccClient().S3Admin

	if err := minioIam.AddUser(ctx, name, "tfacc-outside-secret"); err != nil {
		t.Fatalf("creating user %s outside Terraform: %v", name, err)
	}
	t.Cleanup(func() {
		_ = minioIam.RemoveUser(ctx, name)
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

func testAccMinioUserConfigWithDisable(rName string) string {
	return fmt.Sprintf(`
	resource "minio_iam_user" "test" {
		  name         = %q
		  disable_user = true
		}`, rName)
}

func testAccMinioUserConfigWithoutSecret(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test3" {
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

func testAccMinioUserConfigSetSecret(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test4" {
  secret = "secret1234"
  name   = %q
}
`, rName)
}

func testAccCheckMinioUserExists(n string, res *madmin.UserInfo) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s %s", n, s)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no User name is set")
		}

		minioIam := testAccClient().S3Admin

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
			return fmt.Errorf("not found: %s %s", n, s)
		}

		minioIam := testAccClient().S3Admin

		resp, err := minioIam.GetUserInfo(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error getting user %s", err)
		}

		if rs.Primary.Attributes["status"] != string(madmin.AccountDisabled) || resp.Status != madmin.AccountDisabled {
			return fmt.Errorf("user still enabled: state:%s server:%s", rs.Primary.Attributes["status"], resp.Status)
		}

		return nil
	}
}

func testAccCheckMinioUserAttributes(n string, name string, status string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs := s.RootModule().Resources[n]

		if rs.Primary.Attributes["name"] != name {
			return fmt.Errorf("bad name: %s", name)
		}

		if rs.Primary.Attributes["status"] != status {
			return fmt.Errorf("bad status: %s", status)
		}

		return nil
	}
}

func testAccCheckMinioUserDestroy(s *terraform.State) error {
	minioIam := testAccClient().S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_user" {
			continue
		}

		_, err := minioIam.GetUserInfo(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("user still exists")
		}

	}

	return nil
}

func testAccCheckMinioUserStatus(n string, wantDisableUser string, wantStatus string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if got := rs.Primary.Attributes["disable_user"]; got != wantDisableUser {
			return fmt.Errorf("disable_user = %q, want %q", got, wantDisableUser)
		}

		if got := rs.Primary.Attributes["status"]; got != wantStatus {
			return fmt.Errorf("status = %q, want %q", got, wantStatus)
		}

		resp, err := testAccClient().S3Admin.GetUserInfo(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error getting user %s: %w", rs.Primary.ID, err)
		}

		if string(resp.Status) != wantStatus {
			return fmt.Errorf("server status = %q, want %q", resp.Status, wantStatus)
		}

		return nil
	}
}

func testAccCheckMinioUserSetStatusExternally(username string, status madmin.AccountStatus) error {
	if err := testAccClient().S3Admin.SetUserStatus(context.Background(), username, status); err != nil {
		return fmt.Errorf("error setting user %s status to %s: %w", username, status, err)
	}
	return nil
}

func testAccCheckMinioUserDeleteExternally(username string) error {
	minioIam := testAccClient().S3Admin

	if err := minioIam.RemoveUser(context.Background(), username); err != nil {
		return fmt.Errorf("user could not be deleted: %w", err)
	}

	return nil
}

func testAccCheckMinioUserExfiltrateAccessKey(n string, accessKey *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs := s.RootModule().Resources[n]

		*accessKey = rs.Primary.Attributes["secret"]

		return nil
	}
}

func testAccCheckMinioUserCanLogIn(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs := s.RootModule().Resources[n]

		conn := testAccClient().S3Admin

		userName := rs.Primary.Attributes["name"]

		userInfo, err := conn.GetUserInfo(context.Background(), userName)
		if err != nil {
			return fmt.Errorf("error getting user %s info: %s", userName, err)
		}

		if userInfo.Status != madmin.AccountEnabled {
			return fmt.Errorf("user exists but is not enabled: %s", userName)
		}

		return nil
	}
}

func testAccCheckMinioUserRotatesAccessKey(n string, oldAccessKey *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs := s.RootModule().Resources[n]

		if rs.Primary.Attributes["secret"] == *oldAccessKey {
			return fmt.Errorf("secret has not been rotated")
		}

		return nil
	}
}
