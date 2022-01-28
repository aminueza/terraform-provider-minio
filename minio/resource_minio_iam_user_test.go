package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
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

func testAccCheckMinioUserExfiltrateAccessKey(n string, accessKey *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, _ := s.RootModule().Resources[n]

		*accessKey = rs.Primary.Attributes["secret"]

		return nil
	}
}
func testAccCheckMinioUserCanLogIn(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, _ := s.RootModule().Resources[n]

		// Check if we can log in
		cfg := &S3MinioConfig{
			S3HostPort:   os.Getenv("MINIO_ENDPOINT"),
			S3UserAccess: rs.Primary.Attributes["name"],
			S3UserSecret: rs.Primary.Attributes["secret"],
			S3SSL:        map[string]bool{"true": true, "false": false}[os.Getenv("MINIO_ENABLE_HTTPS")],
		}
		return minioUIwebrpcLogin(cfg)
	}
}

func testAccCheckMinioUserRotatesAccessKey(n string, oldAccessKey *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, _ := s.RootModule().Resources[n]

		if rs.Primary.Attributes["secret"] == *oldAccessKey {
			return fmt.Errorf("Secret has not been rotated")
		}

		return nil
	}
}

// minioUIwebrpcLogin checks if a login is possible to minio.
//
// It does this via webrpc because the User might lack any rights, even listing
// buckets might be forbidden.  This is highly undesirable and should be replaced
// as soon as possible.
func minioUIwebrpcLogin(cfg *S3MinioConfig) error {
	schema := map[bool]string{true: "https", false: "http"}[cfg.S3SSL]
	webrpcData := map[string]interface{}{
		"id":      1,
		"jsonrpc": "2.0",
		"params": map[string]interface{}{
			"username": cfg.S3UserAccess,
			"password": cfg.S3UserSecret,
		},
		"method": "web.Login",
	}
	requestData, _ := json.Marshal(webrpcData)

	client := &http.Client{}
	req, err := http.NewRequest("POST", schema+"://"+cfg.S3HostPort+"/minio/webrpc", strings.NewReader(string(requestData)))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("User-Agent", "Mozilla/5.0") // Server verifies Browser usage
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var loginResponse map[string]interface{}
	err = json.Unmarshal(body, &loginResponse)
	if err != nil {
		if jsonErr, ok := err.(*json.SyntaxError); ok {
			from := int(math.Max(float64(jsonErr.Offset-10.0), 0))
			to := int(math.Min(float64(jsonErr.Offset+50), float64(len(body))))
			problemPart := body[from:to]
			return fmt.Errorf("%w ~ error near '%s' (offset %d)", err, problemPart, jsonErr.Offset)
		}
		return err
	}

	if _, ok := loginResponse["error"]; ok {
		message := loginResponse["error"].(map[string]interface{})["message"]
		return fmt.Errorf("Error logging in: %s", message)
	}

	return nil
}
