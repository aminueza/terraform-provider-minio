package minio

import (
	"context"
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
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyWebhookBasic(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "endpoint", testAccEndpointURL("")),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
			// Import not tested: new targets require MinIO server restart before GetConfigKV can read them.
		},
	})
}

// TestAccMinioNotifyWebhook_deletedOutsideTerraform covers issue #1306. It
// restarts the shared primary MinIO, so it must stay a sequential
// resource.Test: go test starts the parallel tests only after the sequential
// ones finish.
func TestAccMinioNotifyWebhook_deletedOutsideTerraform(t *testing.T) {
	resourceName := "minio_notify_webhook.test"
	name := "tfacc-" + acctest.RandString(6)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyWebhookBasic(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "restart_required", "true"),
				),
			},
			{
				// Restart so the new target becomes visible; the read clears
				// restart_required, the premise the fix uses to tell "waiting on
				// its restart" apart from "the target is gone". The step also
				// asserts the plan is clean once the target is up.
				Config:    testAccMinioNotifyWebhookBasic(name),
				PreConfig: func() { testAccNotifyRestartPreConfig(t) },
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "restart_required", "false"),
				),
			},
			{
				// Delete the target outside Terraform, restart so the removal is
				// applied, and require the plan to notice it is gone.
				Config:             testAccMinioNotifyWebhookBasic(name),
				PreConfig:          func() { testAccNotifyWebhookRemoveOutsideTerraform(t, name) },
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// testAccNotifyRestartPreConfig restarts MinIO so that a notify target written
// through the admin API is applied to the running config. Reuses
// testAccOIDCRestartMinio; the restart plus GetConfigKV helper landing for
// issue #1310 should replace it.
func testAccNotifyRestartPreConfig(t *testing.T) {
	t.Helper()
	if err := testAccOIDCRestartMinio(context.Background(), testAccClient().S3Admin); err != nil {
		t.Fatalf("restarting MinIO: %v", err)
	}
}

// testAccNotifyWebhookRemoveOutsideTerraform deletes the target directly through
// the admin API and restarts MinIO, so the removal is applied to the running
// config, like `mc admin config reset` would be.
func testAccNotifyWebhookRemoveOutsideTerraform(t *testing.T, name string) {
	t.Helper()
	admin := testAccClient().S3Admin
	ctx := context.Background()
	if _, err := admin.DelConfigKV(ctx, notifyConfigKey("notify_webhook", name)); err != nil {
		t.Fatalf("deleting notification target %s outside Terraform: %v", name, err)
	}
	if err := testAccOIDCRestartMinio(ctx, admin); err != nil {
		t.Fatalf("restarting MinIO after deleting %s outside Terraform: %v", name, err)
	}
}

func TestAccMinioNotifyWebhook_update(t *testing.T) {
	resourceName := "minio_notify_webhook.test"
	name := "tfacc-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
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
  endpoint = %[2]q
  enable   = false
}
`, name, testAccEndpointURL(""))
}

func testAccMinioNotifyWebhookWithOptions(name string, queueLimit int) string {
	return fmt.Sprintf(`
resource "minio_notify_webhook" "test" {
  name        = %[1]q
  endpoint    = %[3]q
  enable      = false
  queue_limit = %[2]d
}
`, name, queueLimit, testAccEndpointURL(""))
}
