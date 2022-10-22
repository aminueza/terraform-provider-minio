package minio

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go"
)

func TestServiceAccount_basic(t *testing.T) {
	var serviceAccount madmin.InfoServiceAccountResp

	targetUser := fmt.Sprintf("test-user-%d", acctest.RandInt())
	status := "on"
	resourceName := "minio_iam_service_account.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioServiceAccountDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioServiceAccountConfig(targetUser),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioServiceAccountExists(resourceName, &serviceAccount),
					testAccCheckMinioServiceAccountAttributes(resourceName, targetUser, status),
				),
			},
		},
	})
}

func TestServiceAccount_Disable(t *testing.T) {
	var serviceAccount madmin.InfoServiceAccountResp

	targetUser := fmt.Sprintf("test-user-%d", acctest.RandInt())
	resourceName := "minio_iam_service_account.test1"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioServiceAccountDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioServiceAccountConfigDisabled(targetUser),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioServiceAccountExists(resourceName, &serviceAccount),
					testAccCheckMinioServiceAccountDisabled(resourceName),
				),
			},
		},
	})
}

func TestServiceAccount_RotateAccessKey(t *testing.T) {
	var serviceAccount madmin.InfoServiceAccountResp
	var oldAccessKey string

	targetUser := fmt.Sprintf("test-user-%d", acctest.RandInt())
	resourceName := "minio_iam_service_account.test3"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioServiceAccountDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioServiceAccountConfigWithoutSecret(targetUser),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioServiceAccountExists(resourceName, &serviceAccount),
					testAccCheckMinioServiceAccountExfiltrateAccessKey(resourceName, &oldAccessKey),
					testAccCheckMinioServiceAccountCanLogIn(resourceName),
				),
			},
			{
				Config: testAccMinioServiceAccountConfigUpdateSecret(targetUser),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioServiceAccountExists(resourceName, &serviceAccount),
					testAccCheckMinioServiceAccountRotatesAccessKey(resourceName, &oldAccessKey),
					testAccCheckMinioServiceAccountCanLogIn(resourceName),
				),
			},
		},
	})
}

func testAccMinioServiceAccountConfig(rName string) string {
	return fmt.Sprintf(`
	resource "minio_iam_service_account" "test" {
		  target_user = %q
		}`, rName)
}

func testAccMinioServiceAccountConfigDisabled(rName string) string {
	return fmt.Sprintf(`
	resource "minio_iam_service_account" "test1" {
		  target_user         = %q
		  disable_user = true
		}`, rName)
}

func testAccMinioServiceAccountConfigWithoutSecret(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_service_account" "test3" {
  target_user          = %q
}
`, rName)
}
func testAccMinioServiceAccountConfigUpdateSecret(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_service_account" "test3" {
  update_secret = true
  target_user          = %q
}
`, rName)
}

func testAccCheckMinioServiceAccountExists(n string, res *madmin.InfoServiceAccountResp) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s %s", n, s)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no access_key is set")
		}

		minioIam := testAccProvider.Meta().(*S3MinioClient).S3Admin

		resp, err := minioIam.InfoServiceAccount(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}

		res = &resp

		return nil
	}
}

func testAccCheckMinioServiceAccountDisabled(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s %s", n, s)
		}

		minioIam := testAccProvider.Meta().(*S3MinioClient).S3Admin

		resp, err := minioIam.InfoServiceAccount(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("error getting service account %s", err)
		}

		if rs.Primary.Attributes["status"] != "off" || resp.AccountStatus != "off" {
			return fmt.Errorf("service account still enabled: state:%s server:%s", rs.Primary.Attributes["status"], resp.AccountStatus)
		}

		return nil
	}
}

func testAccCheckMinioServiceAccountAttributes(n string, name string, status string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs := s.RootModule().Resources[n]

		if rs.Primary.Attributes["status"] != status {
			return fmt.Errorf("bad status: %s", status)
		}

		return nil
	}
}

func testAccCheckMinioServiceAccountDestroy(s *terraform.State) error {
	minioIam := testAccProvider.Meta().(*S3MinioClient).S3Admin

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "minio_iam_service_account" {
			continue
		}

		// Try to get service account
		_, err := minioIam.GetUserInfo(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("service account still exists")
		}

	}

	return nil
}

func testAccCheckMinioServiceAccountExfiltrateAccessKey(n string, accessKey *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs := s.RootModule().Resources[n]

		*accessKey = rs.Primary.Attributes["secret_key"]

		return nil
	}
}

func testAccCheckMinioServiceAccountCanLogIn(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs := s.RootModule().Resources[n]

		// Check if we can log in
		cfg := &S3MinioConfig{
			S3HostPort:   os.Getenv("MINIO_ENDPOINT"),
			S3UserAccess: rs.Primary.Attributes["access_key"],
			S3UserSecret: rs.Primary.Attributes["secret_key"],
			S3SSL:        map[string]bool{"true": true, "false": false}[os.Getenv("MINIO_ENABLE_HTTPS")],
		}
		return minioUIwebrpcLogin(cfg)
	}
}

func testAccCheckMinioServiceAccountRotatesAccessKey(n string, oldAccessKey *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs := s.RootModule().Resources[n]

		if rs.Primary.Attributes["secret_key"] == *oldAccessKey {
			return fmt.Errorf("secret has not been rotated")
		}

		return nil
	}
}
