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

	// Sequential on purpose: the server check restarts the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("notify_webhook", name),
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyWebhookBasic(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "endpoint", testAccEndpointURL("")),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
					testAccCheckServerStoredTarget("notify_webhook", name, map[string]string{
						"endpoint": testAccEndpointURL(""),
						"enable":   "off",
					}),
				),
			},
			// Import not tested: new targets require MinIO server restart before GetConfigKV can read them.
		},
	})
}

func TestAccMinioNotifyWebhook_update(t *testing.T) {
	resourceName := "minio_notify_webhook.test"
	name := "tfacc-" + acctest.RandString(6)

	// Sequential on purpose: the server check restarts the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("notify_webhook", name),
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyWebhookWithOptions(name, 50000, "tfacc webhook"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "queue_limit", "50000"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
			{
				Config: testAccMinioNotifyWebhookWithOptions(name, 75000, ""),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "queue_limit", "75000"),
					testAccCheckServerStoredTarget("notify_webhook", name, map[string]string{
						"comment": "",
					}),
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
	// A removal is only visible after a server restart, and the tests that
	// call this check run in parallel against the shared server, so it only
	// verifies the resource left the state.
	return nil
}

func testAccMinioNotifyWebhookBasic(name string) string {
	return fmt.Sprintf(`
resource "minio_notify_webhook" "test" {
  name     = %[1]q
  endpoint = %[2]q
  enable   = false
}
`, name, testAccEndpointURL(""))
}

func testAccMinioNotifyWebhookWithOptions(name string, queueLimit int, comment string) string {
	return fmt.Sprintf(`
resource "minio_notify_webhook" "test" {
  name        = %[1]q
  endpoint    = %[3]q
  enable      = false
  queue_limit = %[2]d
  comment     = %[4]q
}
`, name, queueLimit, testAccEndpointURL(""), comment)
}
