package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMinioNotifyWebhook_basic(t *testing.T) {
	resourceName := "minio_notify_webhook.test"
	name := "tfacc-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyWebhookBasic(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "endpoint", "http://minio:9000"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
			// Import not tested: new targets require MinIO server restart before GetConfigKV can read them.
		},
	})
}

func TestAccMinioNotifyWebhook_update(t *testing.T) {
	resourceName := "minio_notify_webhook.test"
	name := "tfacc-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyWebhookWithOptions(name, 50000),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "queue_limit", "50000"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
			{
				Config: testAccMinioNotifyWebhookWithOptions(name, 75000),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "queue_limit", "75000"),
				),
			},
		},
	})
}


func testAccCheckNotifyTargetExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("no notification target ID is set")
		}
		return nil
	}
}

func testAccCheckNotifyTargetDestroy(s *terraform.State) error {
	// Config subsystems persist with defaults after deletion.
	return nil
}

func testAccMinioNotifyWebhookBasic(name string) string {
	return fmt.Sprintf(`
resource "minio_notify_webhook" "test" {
  name     = %[1]q
  endpoint = "http://minio:9000"
  enable   = false
}
`, name)
}

func testAccMinioNotifyWebhookWithOptions(name string, queueLimit int) string {
	return fmt.Sprintf(`
resource "minio_notify_webhook" "test" {
  name        = %[1]q
  endpoint    = "http://minio:9000"
  enable      = false
  queue_limit = %[2]d
}
`, name, queueLimit)
}
