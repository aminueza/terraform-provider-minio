package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/minio/madmin-go/v3"
	"gotest.tools/v3/assert"
)

func TestServiceAccount_basic(t *testing.T) {
	var serviceAccount madmin.InfoServiceAccountResp

	targetUser := "minio"
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
					testAccCheckMinioServiceAccountExists(testAccProvider, resourceName, &serviceAccount),
					testAccCheckMinioServiceAccountAttributes(resourceName, targetUser, status),
					resource.TestCheckResourceAttr(resourceName, "target_user", targetUser),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"update_secret", "secret_key"},
			},
		},
	})
}

func TestServiceAccount_Disable(t *testing.T) {
	var serviceAccount madmin.InfoServiceAccountResp

	targetUser := "minio"
	resourceName := "minio_iam_service_account.test1"

	provider := newProvider()
	providers := map[string]func() (*schema.Provider, error){
		"minio": func() (*schema.Provider, error) {
			return provider, nil
		},
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providers,
		CheckDestroy:      testAccCheckMinioServiceAccountDestroyWithProvider(provider),
		Steps: []resource.TestStep{
			{
				Config: testAccMinioServiceAccountConfigDisabled(targetUser),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioServiceAccountExists(provider, resourceName, &serviceAccount),
					testAccCheckMinioServiceAccountDisabled(provider, resourceName),
				),
			},
		},
	})
}

func TestServiceAccount_RotateAccessKey(t *testing.T) {
	var serviceAccount madmin.InfoServiceAccountResp
	var oldAccessKey string

	targetUser := "minio"
	resourceName := "minio_iam_service_account.test3"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioServiceAccountDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioServiceAccountConfigWithoutSecret(targetUser),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioServiceAccountExists(testAccProvider, resourceName, &serviceAccount),
					testAccCheckMinioServiceAccountExfiltrateAccessKey(resourceName, &oldAccessKey),
					testAccCheckMinioServiceAccountCanLogIn(resourceName),
				),
			},
			{
				Config: testAccMinioServiceAccountConfigUpdateSecret(targetUser),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioServiceAccountExists(testAccProvider, resourceName, &serviceAccount),
					testAccCheckMinioServiceAccountRotatesAccessKey(resourceName, &oldAccessKey),
					testAccCheckMinioServiceAccountCanLogIn(resourceName),
				),
			},
		},
	})
}
func TestServiceAccount_Policy(t *testing.T) {
	var serviceAccount madmin.InfoServiceAccountResp
	var oldAccessKey string

	targetUser := "minio"
	resourceName := "minio_iam_service_account.test4"
	policy1 := "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":[\"s3:ListAllMyBuckets\"],\"Resource\":[\"arn:aws:s3:::*\"]}]}"
	policy2 := "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":[\"s3:*\"],\"Resource\":[\"arn:aws:s3:::*\"]}]}"

	targetUser2 := "test"
	resourceName2 := "minio_iam_service_account.test_service_account"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioServiceAccountDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioServiceAccountConfigPolicy(targetUser),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioServiceAccountExists(testAccProvider, resourceName, &serviceAccount),
					testAccCheckMinioServiceAccountExfiltrateAccessKey(resourceName, &oldAccessKey),
					testAccCheckMinioServiceAccountCanLogIn(resourceName),
					testAccCheckMinioServiceAccountPolicy(resourceName, policy1),
				),
			},
			{
				Config: testAccMinioServiceAccountConfigUpdatePolicy(targetUser),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioServiceAccountExists(testAccProvider, resourceName, &serviceAccount),
					testAccCheckMinioServiceAccountRotatesAccessKey(resourceName, &oldAccessKey),
					testAccCheckMinioServiceAccountCanLogIn(resourceName),
					testAccCheckMinioServiceAccountPolicy(resourceName, policy2),
				),
			},
			{
				Config: testAccMinioServiceAccountWithUserPolicy(targetUser2),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioServiceAccountExists(testAccProvider, resourceName2, &serviceAccount),
				),
			},
			{
				ResourceName:            resourceName2,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"update_secret", "secret_key"},
			},
		},
	})
}

func TestParseUserFromParentUser(t *testing.T) {
	assert.Equal(t, "minio-user", parseUserFromParentUser("minio-user"))
	assert.Equal(t, "minio-user", parseUserFromParentUser("CN = minio-user, DC=example,DC=org"))
	assert.Equal(t, "minio-user", parseUserFromParentUser("cn=minio-user, DC=example"))
}

func TestServiceAccount_NameDesc(t *testing.T) {
	var serviceAccount madmin.InfoServiceAccountResp

	targetUser := "minio"
	resourceName := "minio_iam_service_account.test"
	name := "svc-account"
	description := "A service account"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioServiceAccountDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioServiceAccountConfig(targetUser),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioServiceAccountExists(testAccProvider, resourceName, &serviceAccount),
				),
			},
			{
				Config: testAccMinioServiceAccountConfigUpdateNameDesc(targetUser, name, description),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioServiceAccountExists(testAccProvider, resourceName, &serviceAccount),
					testAccCheckMinioServiceAccountNameDesc(resourceName, name, description),
				),
			},
		},
	})
}

func TestServiceAccount_Expiration(t *testing.T) {
	var serviceAccount madmin.InfoServiceAccountResp

	targetUser := "minio"
	resourceName := "minio_iam_service_account.test"
	expiration := time.Now().Add(time.Hour * 1).UTC().Format(time.RFC3339)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioServiceAccountDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioServiceAccountConfig(targetUser),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioServiceAccountExists(testAccProvider, resourceName, &serviceAccount),
					testAccCheckMinioServiceAccountExpiration(resourceName, ""),
				),
			},
			{
				Config: testAccMinioServiceAccountConfigUpdateExpiration(targetUser, expiration),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioServiceAccountExists(testAccProvider, resourceName, &serviceAccount),
					testAccCheckMinioServiceAccountExpiration(resourceName, expiration),
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
  target_user = %q
}
`, rName)
}
func testAccMinioServiceAccountConfigUpdateSecret(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_service_account" "test3" {
  update_secret = true
  target_user   = %q
}
`, rName)
}
func testAccMinioServiceAccountConfigPolicy(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_service_account" "test4" {
  target_user   = %q
  policy = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Action\":[\"s3:ListAllMyBuckets\"],\"Effect\":\"Allow\",\"Resource\":[\"arn:aws:s3:::*\"]}]}"
}
`, rName)
}
func testAccMinioServiceAccountConfigUpdatePolicy(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_service_account" "test4" {
  target_user   = %q
  update_secret = true
  policy = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Action\":[\"s3:*\"],\"Effect\":\"Allow\",\"Resource\":[\"arn:aws:s3:::*\"]}]}"
}
`, rName)
}
func testAccMinioServiceAccountWithUserPolicy(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test_user" {
  secret = "secret1234"
  name   = %q
}

resource "minio_iam_policy" "test_policy" {
  name   = "state-terraform-s3"
  policy = <<EOF
{
	"Version":"2012-10-17",
	"Statement": [
		{
			"Sid":"ListAllBucket",
			"Effect": "Allow",
			"Action": ["s3:PutObject"],
			"Principal":"*",
			"Resource": "arn:aws:s3:::test_bucket/*"
		}
	]
}
EOF
}

resource "minio_iam_user_policy_attachment" "test_policy_attachment" {
  user_name   = minio_iam_user.test_user.id
  policy_name = minio_iam_policy.test_policy.id
}

resource "minio_iam_service_account" "test_service_account" {
  target_user   = minio_iam_user.test_user.id
}
`, rName)
}

func testAccMinioServiceAccountConfigUpdateNameDesc(rName string, name string, description string) string {
	return fmt.Sprintf(`
	resource "minio_iam_service_account" "test" {
		  target_user = %q
      name = %q
      description = %q
		}`, rName, name, description)
}

func testAccMinioServiceAccountConfigUpdateExpiration(rName string, expiration string) string {
	return fmt.Sprintf(`
	resource "minio_iam_service_account" "test" {
		  target_user = %q
      expiration = %q
		}`, rName, expiration)
}

func testAccCheckMinioServiceAccountExists(provider *schema.Provider, n string, res *madmin.InfoServiceAccountResp) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s %s", n, s)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no access_key is set")
		}

		minioIam := provider.Meta().(*S3MinioClient).S3Admin

		resp, err := minioIam.InfoServiceAccount(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}

		*res = resp

		return nil
	}
}

func testAccCheckMinioServiceAccountDisabled(provider *schema.Provider, n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s %s", n, s)
		}

		minioIam := provider.Meta().(*S3MinioClient).S3Admin

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

func testAccCheckMinioServiceAccountAttributes(n string, _ string, status string) resource.TestCheckFunc {
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

func testAccCheckMinioServiceAccountDestroyWithProvider(provider *schema.Provider) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		minioIam := provider.Meta().(*S3MinioClient).S3Admin

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
			S3APISignature: "v4",
			S3HostPort:     os.Getenv("MINIO_ENDPOINT"),
			S3UserAccess:   rs.Primary.Attributes["access_key"],
			S3UserSecret:   rs.Primary.Attributes["secret_key"],
			S3SSL:          map[string]bool{"true": true, "false": false}[os.Getenv("MINIO_ENABLE_HTTPS")],
		}
		client, err := cfg.NewClient()
		if err != nil {
			return err
		}
		_, err = client.(*S3MinioClient).S3Client.ListBuckets(context.Background())
		return err
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

func testAccCheckMinioServiceAccountPolicy(n string, expectedPolicy string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		v, ok := rs.Primary.Attributes["policy"]
		if !ok {
			return fmt.Errorf("%s: Attribute 'policy' not found", n)
		}

		var actual, expected interface{}
		_ = json.Unmarshal([]byte(expectedPolicy), &expected)
		_ = json.Unmarshal([]byte(v), &actual)
		diff := cmp.Diff(actual, expected)
		if diff != "" {
			return fmt.Errorf("%s: mismatch (-want +got):\n%s", n, diff)
		}

		return nil
	}
}

func testAccCheckMinioServiceAccountNameDesc(n string, name string, description string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs := s.RootModule().Resources[n]

		if rs.Primary.Attributes["name"] != name {
			return fmt.Errorf("bad name: %s", name)
		}
		if rs.Primary.Attributes["description"] != description {
			return fmt.Errorf("bad description: %s", description)
		}

		return nil
	}
}

func testAccCheckMinioServiceAccountExpiration(n string, expiration string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs := s.RootModule().Resources[n]

		if rs.Primary.Attributes["expiration"] != expiration {
			return fmt.Errorf("bad expiration: %s", expiration)
		}

		return nil
	}
}
