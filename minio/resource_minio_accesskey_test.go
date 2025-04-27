package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccMinioAccessKey_basic(t *testing.T) {
	rName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_accesskey.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioAccessKeyConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "user", rName),
					resource.TestCheckResourceAttr(resourceName, "status", "enabled"),
					resource.TestCheckResourceAttrSet(resourceName, "access_key"),
					resource.TestCheckResourceAttrSet(resourceName, "secret_key"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"secret_key"},
			},
		},
	})
}

func TestAccMinioAccessKey_update(t *testing.T) {
	rName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_accesskey.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioAccessKeyConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "user", rName),
					resource.TestCheckResourceAttr(resourceName, "status", "enabled"),
				),
			},
			{
				Config: testAccMinioAccessKeyConfigDisabled(rName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "user", rName),
					resource.TestCheckResourceAttr(resourceName, "status", "disabled"),
				),
			},
		},
	})
}

func testAccMinioAccessKeyConfig(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test" {
  name = %q
}

resource "minio_accesskey" "test" {
  user = minio_iam_user.test.name
  status = "enabled"
}
`, rName)
}

func testAccMinioAccessKeyConfigDisabled(rName string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test" {
  name = %q
}

resource "minio_accesskey" "test" {
  user = minio_iam_user.test.name
  status = "disabled"
}
`, rName)
}

func TestAccMinioAccessKey_customKeys(t *testing.T) {
	rName := acctest.RandomWithPrefix("tf-acc-test")
	resourceName := "minio_accesskey.test"
	customAccessKey := acctest.RandString(20)
	customSecretKey := acctest.RandString(40)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioAccessKeyConfigCustomKeys(rName, customAccessKey, customSecretKey),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "user", rName),
					resource.TestCheckResourceAttr(resourceName, "access_key", customAccessKey),
					resource.TestCheckResourceAttr(resourceName, "secret_key", customSecretKey),
				),
			},
		},
	})
}

func testAccMinioAccessKeyConfigCustomKeys(rName, accessKey, secretKey string) string {
	return fmt.Sprintf(`
resource "minio_iam_user" "test" {
  name = %q
}

resource "minio_accesskey" "test" {
  user = minio_iam_user.test.name
  access_key = %q
  secret_key = %q
}
`, rName, accessKey, secretKey)
}
