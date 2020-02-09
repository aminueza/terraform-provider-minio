package minio

import (
	"fmt"
	"testing"

	"github.com/aminueza/terraform-minio-provider/madmin"
	"github.com/hashicorp/terraform/helper/acctest"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
)

func TestValidateMinioIamGroupName(t *testing.T) {
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
		_, err := validateMinioIamGroupName(minioName, "name")
		if len(err) != 0 {
			t.Fatalf("%q should be a valid IAM Group name: %q", minioName, err)
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
		_, err := validateMinioIamGroupName(minioName, "name")
		if len(err) == 0 {
			t.Fatalf("%q should be an invalid IAM Group name", minioName)
		}
	}
}

func TestAccAWSGroup_Basic(t *testing.T) {
	var conf madmin.GroupDesc

	groupName := fmt.Sprintf("tf-acc-group-basic-%d", acctest.RandInt())
	groupName2 := fmt.Sprintf("tf-acc-group-basic-2-%d", acctest.RandInt())
	status1 := "enabled"
	status2 := "disabled"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckMinioUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioGroupConfig(groupName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupExists("minio_iam_group.test", &conf),
					testAccCheckMinioGroupAttributes(&conf, groupName, status1),
				),
			},
			{
				Config: testAccMinioGroupConfig2(groupName2),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioGroupExists("minio_iam_group.test2", &conf),
					testAccCheckMinioGroupDisable(&conf, groupName2, status2),
				),
			},
		},
	})
}

func testAccMinioGroupConfig(groupName string) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "test" {
  name = "%s"
}
`, groupName)
}

func testAccMinioGroupConfig2(groupName string) string {
	return fmt.Sprintf(`
resource "minio_iam_group" "test2" {
  name = "%s"
  disable_group = "true"
}
`, groupName)
}

func testAccCheckMinioGroupExists(n string, res *madmin.GroupDesc) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No Group name is set")
		}

		minioIam := testAccProvider.Meta().(*S3MinioClient).S3Admin

		resp, err := minioIam.GetGroupDescription(rs.Primary.ID)
		if err != nil {
			return err
		}

		*res = *resp

		return nil
	}
}

func testAccCheckMinioGroupAttributes(group *madmin.GroupDesc, name string, status string) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if group.Name != name {
			return fmt.Errorf("Bad name: %s", group.Name)
		}

		if group.Status != status {
			return fmt.Errorf("Bad status: %s", group.Status)
		}

		return nil
	}
}

func testAccCheckMinioGroupDisable(group *madmin.GroupDesc, name string, status string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		minioIam := testAccProvider.Meta().(*S3MinioClient).S3Admin

		if group.Name != name {
			return fmt.Errorf("Bad name: %s", group.Name)
		}

		err := minioIam.SetGroupStatus(group.Name, madmin.GroupStatus("disabled"))
		if err != nil {
			return err
		}

		resp, err := minioIam.GetGroupDescription(group.Name)
		if err != nil {
			return err
		}

		if resp.Status != status {
			return fmt.Errorf("Bad status: %s", resp.Status)
		}

		return nil
	}
}
