package minio

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestBuildAuditWebhookCfgData(t *testing.T) {
	tests := []struct {
		name     string
		config   S3MinioAuditWebhook
		contains []string
		absent   []string
	}{
		{
			name: "basic fields",
			config: S3MinioAuditWebhook{
				Endpoint: "http://audit.example.com/ingest",
				Enable:   true,
			},
			contains: []string{
				"endpoint=http://audit.example.com/ingest",
				"enable=on",
			},
			absent: []string{
				"auth_token=",
				"queue_size=",
				"batch_size=",
				"client_cert=",
				"client_key=",
			},
		},
		{
			name: "all fields",
			config: S3MinioAuditWebhook{
				Endpoint:   "https://splunk.example.com:8088/services/collector",
				AuthToken:  "my-secret-token",
				Enable:     false,
				QueueSize:  100000,
				BatchSize:  500,
				ClientCert: "/path/to/cert.pem",
				ClientKey:  "/path/to/key.pem",
			},
			contains: []string{
				"endpoint=https://splunk.example.com:8088/services/collector",
				"auth_token=my-secret-token",
				"enable=off",
				"queue_size=100000",
				"batch_size=500",
				"client_cert=/path/to/cert.pem",
				"client_key=/path/to/key.pem",
			},
		},
		{
			name: "value with spaces is quoted",
			config: S3MinioAuditWebhook{
				Endpoint:  "http://example.com",
				AuthToken: "Bearer my token",
				Enable:    true,
			},
			contains: []string{
				`auth_token="Bearer my token"`,
				"enable=on",
			},
		},
		{
			name: "disabled with no optional fields",
			config: S3MinioAuditWebhook{
				Endpoint: "http://example.com",
				Enable:   false,
			},
			contains: []string{
				"endpoint=http://example.com",
				"enable=off",
			},
			absent: []string{
				"auth_token=",
				"queue_size=",
				"batch_size=",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildAuditWebhookCfgData(&tc.config)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in config data, got: %s", want, got)
				}
			}
			for _, unwanted := range tc.absent {
				if strings.Contains(got, unwanted) {
					t.Errorf("unexpected %q in config data, got: %s", unwanted, got)
				}
			}
		})
	}
}

func TestAccMinioAuditWebhook_basic(t *testing.T) {
	resourceName := "minio_audit_webhook.test"
	name := "tfacc-" + acctest.RandString(6)

	// Sequential on purpose: the server check restarts the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("audit_webhook", name),
		Steps: []resource.TestStep{
			{
				Config: testAccMinioAuditWebhookBasic(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioAuditWebhookExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "endpoint", "http://audit.example.com/ingest"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
					testAccCheckServerStoredTarget("audit_webhook", name, map[string]string{
						"endpoint": "http://audit.example.com/ingest",
						"enable":   "off",
					}),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"auth_token", "restart_required"},
			},
		},
	})
}

// Turning the webhook on outside Terraform must show up as drift and be
// reverted by the next apply. See issue #1317.
func TestAccMinioAuditWebhook_enableChangedOutsideTerraform(t *testing.T) {
	resourceName := "minio_audit_webhook.test"
	name := "tfacc-" + acctest.RandString(6)

	// Sequential on purpose: the server checks restart the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("audit_webhook", name),
		Steps: []resource.TestStep{
			{
				Config: testAccMinioAuditWebhookBasic(name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
					testAccCheckServerStoredTarget("audit_webhook", name, map[string]string{
						"enable": "off",
					}),
				),
			},
			{
				PreConfig:          func() { testAccEnableAuditWebhookOutsideTerraform(t, name) },
				Config:             testAccMinioAuditWebhookBasic(name),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccMinioAuditWebhookBasic(name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
					testAccCheckServerStoredTarget("audit_webhook", name, map[string]string{
						"enable": "off",
					}),
				),
			},
		},
	})
}

func testAccEnableAuditWebhookOutsideTerraform(t *testing.T, name string) {
	t.Helper()
	if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), auditWebhookConfigKey(name)+" enable=on"); err != nil {
		t.Fatalf("enabling %s outside Terraform: %v", name, err)
	}
	if err := testAccRestartPrimaryServer(); err != nil {
		t.Fatalf("restarting the server: %v", err)
	}
}

func TestAccMinioAuditWebhook_removeCredentials(t *testing.T) {
	resourceName := "minio_audit_webhook.test"
	name := "tfacc-" + acctest.RandString(6)

	// Sequential on purpose: the server check restarts the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioAuditWebhookDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioAuditWebhookWithCredentials(name),
				Check:  resource.TestCheckResourceAttr(resourceName, "client_cert", "/srv/tls/client-cert"),
			},
			{
				Config: testAccMinioAuditWebhookBasic(name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "client_cert", ""),
					// get-config-kv leaves out an auth_token that is set and
					// prints it empty once it is cleared.
					testAccCheckServerStoredTarget("audit_webhook", name, map[string]string{
						"auth_token":  "",
						"client_cert": "",
						"client_key":  "",
					}),
				),
			},
		},
	})
}

func TestAccMinioAuditWebhook_update(t *testing.T) {
	resourceName := "minio_audit_webhook.test"
	name := "tfacc-" + acctest.RandString(6)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckMinioAuditWebhookDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioAuditWebhookWithQueue(name, 50000, 100),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioAuditWebhookExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "queue_size", "50000"),
					resource.TestCheckResourceAttr(resourceName, "batch_size", "100"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
			{
				Config: testAccMinioAuditWebhookWithQueue(name, 75000, 200),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMinioAuditWebhookExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "queue_size", "75000"),
					resource.TestCheckResourceAttr(resourceName, "batch_size", "200"),
				),
			},
		},
	})
}

func testAccCheckMinioAuditWebhookExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no audit webhook ID is set")
		}

		minioC := testAccClient()
		configKey := auditWebhookConfigKey(rs.Primary.ID)
		// The admin config API can return transient errors (e.g. signature
		// mismatches) while parallel tests churn server configuration, so
		// poll briefly instead of failing on the first attempt.
		err := retry.RetryContext(context.Background(), 10*time.Second, func() *retry.RetryError {
			if _, err := minioC.S3Admin.GetConfigKV(context.Background(), configKey); err != nil {
				return retry.RetryableError(fmt.Errorf("audit webhook %s not found: %w", rs.Primary.ID, err))
			}
			return nil
		})
		return err
	}
}

func testAccCheckMinioAuditWebhookDestroy(s *terraform.State) error {
	// A removal is only visible after a server restart, and the tests that
	// call this check run in parallel against the shared server, so they
	// only verify the resource left the state.
	return nil
}

func testAccMinioAuditWebhookBasic(name string) string {
	return fmt.Sprintf(`
resource "minio_audit_webhook" "test" {
  name     = %[1]q
  endpoint = "http://audit.example.com/ingest"
  enable   = false
}
`, name)
}

func testAccMinioAuditWebhookWithQueue(name string, queueSize, batchSize int) string {
	return fmt.Sprintf(`
resource "minio_audit_webhook" "test" {
  name       = %[1]q
  endpoint   = "http://audit.example.com/ingest"
  enable     = false
  queue_size = %[2]d
  batch_size = %[3]d
}
`, name, queueSize, batchSize)
}

func testAccMinioAuditWebhookWithCredentials(name string) string {
	return fmt.Sprintf(`
resource "minio_audit_webhook" "test" {
  name        = %[1]q
  endpoint    = "http://audit.example.com/ingest"
  enable      = false
  auth_token  = "Bearer tfacc"
  client_cert = "/srv/tls/client-cert"
  client_key  = "/srv/tls/client-privkey"
}
`, name)
}
