package minio

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// TestAccMinioServiceAction_freeze is skipped by default because freezing
// the MinIO server suspends all S3 API calls, which disrupts other
// concurrent acceptance tests running against the shared instance.
// To run manually: SKIP_FREEZE_TEST=0 TF_ACC=1 go test -v ./minio -run TestAccMinioServiceAction_freeze
func TestAccMinioServiceAction_freeze(t *testing.T) {
	if os.Getenv("SKIP_FREEZE_TEST") != "0" {
		t.Skip("skipping freeze test (disruptive to parallel tests against the shared MinIO instance); set SKIP_FREEZE_TEST=0 to enable")
	}

	action := fmt.Sprintf("tfacc-service-action-%d", acctest.RandInt())
	resourceName := "minio_service_action.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioServiceActionConfig(action, "freeze"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(resourceName, "action", regexp.MustCompile(`^freeze$`)),
					resource.TestMatchResourceAttr(resourceName, "executed_at", regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)),
					resource.TestMatchResourceAttr(resourceName, "result", regexp.MustCompile(`^MinIO cluster frozen`)),
					resource.TestMatchResourceAttr(resourceName, "id", regexp.MustCompile(`^freeze-.*$`)),
				),
			},
		},
	})
}

// TestAccMinioServiceAction_unfreeze is skipped by default because it
// requires the cluster to be frozen first, and running freeze/unfreeze
// against the shared instance disrupts other concurrent tests.
// To run manually: SKIP_UNFREEZE_TEST=0 TF_ACC=1 go test -v ./minio -run TestAccMinioServiceAction_unfreeze
func TestAccMinioServiceAction_unfreeze(t *testing.T) {
	if os.Getenv("SKIP_UNFREEZE_TEST") != "0" {
		t.Skip("skipping unfreeze test (disruptive to parallel tests against the shared MinIO instance); set SKIP_UNFREEZE_TEST=0 to enable")
	}

	action := fmt.Sprintf("tfacc-service-action-%d", acctest.RandInt())
	resourceName := "minio_service_action.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioServiceActionConfig(action, "unfreeze"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(resourceName, "action", regexp.MustCompile(`^unfreeze$`)),
					resource.TestMatchResourceAttr(resourceName, "executed_at", regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)),
					resource.TestMatchResourceAttr(resourceName, "result", regexp.MustCompile(`^MinIO cluster unfrozen`)),
					resource.TestMatchResourceAttr(resourceName, "id", regexp.MustCompile(`^unfreeze-.*$`)),
				),
			},
		},
	})
}

// TestAccMinioServiceAction_restart is skipped by default because restarting
// the MinIO server would disrupt other concurrent acceptance tests.
// To run manually: TF_ACC=1 go test -v ./minio -run TestAccMinioServiceAction_restart
func TestAccMinioServiceAction_restart(t *testing.T) {
	if os.Getenv("SKIP_RESTART_TEST") != "0" {
		t.Skip("skipping restart test (disruptive to shared MinIO instance); set SKIP_RESTART_TEST=0 to enable")
	}

	action := fmt.Sprintf("tfacc-service-action-%d", acctest.RandInt())
	resourceName := "minio_service_action.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioServiceActionConfig(action, "restart"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(resourceName, "action", regexp.MustCompile(`^restart$`)),
					resource.TestMatchResourceAttr(resourceName, "executed_at", regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)),
					resource.TestMatchResourceAttr(resourceName, "result", regexp.MustCompile(`^MinIO cluster restarted`)),
					resource.TestMatchResourceAttr(resourceName, "id", regexp.MustCompile(`^restart-.*$`)),
				),
			},
		},
	})
}

// TestAccMinioServiceAction_stop is skipped by default because stopping
// the MinIO server would disrupt other concurrent acceptance tests.
// To run manually: TF_ACC=1 go test -v ./minio -run TestAccMinioServiceAction_stop
func TestAccMinioServiceAction_stop(t *testing.T) {
	if os.Getenv("SKIP_STOP_TEST") != "0" {
		t.Skip("skipping stop test (disruptive to shared MinIO instance); set SKIP_STOP_TEST=0 to enable")
	}

	action := fmt.Sprintf("tfacc-service-action-%d", acctest.RandInt())
	resourceName := "minio_service_action.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioServiceActionConfig(action, "stop"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(resourceName, "action", regexp.MustCompile(`^stop$`)),
					resource.TestMatchResourceAttr(resourceName, "executed_at", regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)),
					resource.TestMatchResourceAttr(resourceName, "result", regexp.MustCompile(`^MinIO cluster stopped`)),
					resource.TestMatchResourceAttr(resourceName, "id", regexp.MustCompile(`^stop-.*$`)),
				),
			},
		},
	})
}

func testAccMinioServiceActionConfig(actionName string, action string) string {
	return fmt.Sprintf(`
resource "minio_service_action" "test" {
  action   = "%s"
  triggers = {
    name = "%s"
  }
}
`, action, actionName)
}
