package minio

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// The retry and timeout keys are dynamic on both webhook subsystems, so
// neither applying them nor changing them outside Terraform needs a restart;
// only testAccCheckServerStoredTarget restarts, to read back what was stored.
// See issue #1370.

func testAccMinioAuditWebhookRetry(name string, withRetry bool) string {
	if withRetry {
		// The server mkdirs the queue dir when it builds the target, even for
		// a disabled one, so it must be writable: an unwritable path makes
		// MinIO itself panic.
		return fmt.Sprintf(`
resource "minio_audit_webhook" "test" {
  name           = %q
  endpoint       = "http://audit.example.com/ingest"
  enable         = false
  max_retry      = 3
  retry_interval = "10s"
  http_timeout   = "10s"
  queue_dir      = "/tmp/tfacc-audit-%s"
}
`, name, name)
	}
	return fmt.Sprintf(`
resource "minio_audit_webhook" "test" {
  name     = %q
  endpoint = "http://audit.example.com/ingest"
  enable   = false
}
`, name)
}

func testAccMinioLoggerWebhookRetry(name string, withRetry bool) string {
	if withRetry {
		return fmt.Sprintf(`
resource "minio_logger_webhook" "test" {
  name           = %q
  endpoint       = "http://logger.example.com/logs"
  enable         = false
  max_retry      = 3
  retry_interval = "10s"
  http_timeout   = "10s"
}
`, name)
	}
	return fmt.Sprintf(`
resource "minio_logger_webhook" "test" {
  name     = %q
  endpoint = "http://logger.example.com/logs"
  enable   = false
}
`, name)
}

func testAccChangeWebhookRetryOutsideTerraform(t *testing.T, subsystem, name string) {
	t.Helper()
	target := notifyConfigKey(subsystem, name) + " max_retry=7 retry_interval=5s http_timeout=8s"
	if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), target); err != nil {
		t.Fatalf("changing %s outside Terraform: %v", subsystem, err)
	}
}

func TestAccMinioAuditWebhook_retryFields(t *testing.T) {
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
				Config: testAccMinioAuditWebhookRetry(name, true),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "max_retry", "3"),
					resource.TestCheckResourceAttr(resourceName, "retry_interval", "10s"),
					resource.TestCheckResourceAttr(resourceName, "http_timeout", "10s"),
					resource.TestCheckResourceAttr(resourceName, "queue_dir", "/tmp/tfacc-audit-"+name),
					testAccCheckServerStoredTarget("audit_webhook", name, map[string]string{
						"max_retry":      "3",
						"retry_interval": "10s",
						"http_timeout":   "10s",
						"queue_dir":      "/tmp/tfacc-audit-" + name,
					}),
				),
			},
			{Config: testAccMinioAuditWebhookRetry(name, true), PlanOnly: true},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"auth_token", "restart_required"},
			},
			{
				PreConfig:          func() { testAccChangeWebhookRetryOutsideTerraform(t, "audit_webhook", name) },
				Config:             testAccMinioAuditWebhookRetry(name, true),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			// The refresh must map every changed key into state, not just
			// enough of them to make the plan non-empty.
			{
				RefreshState:       true,
				ExpectNonEmptyPlan: true, // the drift above is left unapplied on purpose
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "max_retry", "7"),
					resource.TestCheckResourceAttr(resourceName, "retry_interval", "5s"),
					resource.TestCheckResourceAttr(resourceName, "http_timeout", "8s"),
				),
			},
			{
				Config: testAccMinioAuditWebhookRetry(name, true),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "max_retry", "3"),
					resource.TestCheckResourceAttr(resourceName, "retry_interval", "10s"),
					resource.TestCheckResourceAttr(resourceName, "http_timeout", "10s"),
					testAccCheckServerStoredTarget("audit_webhook", name, map[string]string{
						"max_retry":      "3",
						"retry_interval": "10s",
						"http_timeout":   "10s",
						"queue_dir":      "/tmp/tfacc-audit-" + name,
					}),
				),
			},
			{
				// Removing the fields writes MinIO's own defaults back, never
				// an empty value: an empty duration fails to parse once the
				// target is enabled.
				Config: testAccMinioAuditWebhookRetry(name, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "max_retry", "0"),
					resource.TestCheckResourceAttr(resourceName, "retry_interval", "3s"),
					resource.TestCheckResourceAttr(resourceName, "http_timeout", "5s"),
					resource.TestCheckResourceAttr(resourceName, "queue_dir", ""),
					testAccCheckServerStoredTarget("audit_webhook", name, map[string]string{
						"max_retry":      "0",
						"retry_interval": "3s",
						"http_timeout":   "5s",
						"queue_dir":      "",
					}),
				),
			},
			{Config: testAccMinioAuditWebhookRetry(name, false), PlanOnly: true},
			{
				// With the fields left out, drift on the server is still
				// corrected back to the defaults: max_retry through the
				// native diff, the durations through the planned default.
				PreConfig: func() {
					target := auditWebhookConfigKey(name) + " max_retry=7 retry_interval=30s"
					if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), target); err != nil {
						t.Fatalf("drifting %s: %v", name, err)
					}
				},
				Config: testAccMinioAuditWebhookRetry(name, false),
				Check: testAccCheckServerStoredTarget("audit_webhook", name, map[string]string{
					"max_retry":      "0",
					"retry_interval": "3s",
					"http_timeout":   "5s",
				}),
			},
		},
	})
}

func TestAccMinioLoggerWebhook_retryFields(t *testing.T) {
	resourceName := "minio_logger_webhook.test"
	name := "tfacc-" + acctest.RandString(6)

	// Sequential on purpose: the server checks restart the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("logger_webhook", name),
		Steps: []resource.TestStep{
			{
				Config: testAccMinioLoggerWebhookRetry(name, true),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "max_retry", "3"),
					resource.TestCheckResourceAttr(resourceName, "retry_interval", "10s"),
					resource.TestCheckResourceAttr(resourceName, "http_timeout", "10s"),
					testAccCheckServerStoredTarget("logger_webhook", name, map[string]string{
						"max_retry":      "3",
						"retry_interval": "10s",
						"http_timeout":   "10s",
					}),
				),
			},
			{Config: testAccMinioLoggerWebhookRetry(name, true), PlanOnly: true},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"auth_token", "restart_required"},
			},
			{
				PreConfig:          func() { testAccChangeWebhookRetryOutsideTerraform(t, "logger_webhook", name) },
				Config:             testAccMinioLoggerWebhookRetry(name, true),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			// The refresh must map every changed key into state, not just
			// enough of them to make the plan non-empty.
			{
				RefreshState:       true,
				ExpectNonEmptyPlan: true, // the drift above is left unapplied on purpose
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "max_retry", "7"),
					resource.TestCheckResourceAttr(resourceName, "retry_interval", "5s"),
					resource.TestCheckResourceAttr(resourceName, "http_timeout", "8s"),
				),
			},
			{
				Config: testAccMinioLoggerWebhookRetry(name, true),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "max_retry", "3"),
					resource.TestCheckResourceAttr(resourceName, "retry_interval", "10s"),
					resource.TestCheckResourceAttr(resourceName, "http_timeout", "10s"),
					testAccCheckServerStoredTarget("logger_webhook", name, map[string]string{
						"max_retry":      "3",
						"retry_interval": "10s",
						"http_timeout":   "10s",
					}),
				),
			},
			{
				// Same rule as the audit webhook: removals go back to
				// MinIO's 0 and 3s and 5s defaults.
				Config: testAccMinioLoggerWebhookRetry(name, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "max_retry", "0"),
					resource.TestCheckResourceAttr(resourceName, "retry_interval", "3s"),
					resource.TestCheckResourceAttr(resourceName, "http_timeout", "5s"),
					testAccCheckServerStoredTarget("logger_webhook", name, map[string]string{
						"max_retry":      "0",
						"retry_interval": "3s",
						"http_timeout":   "5s",
					}),
				),
			},
			{Config: testAccMinioLoggerWebhookRetry(name, false), PlanOnly: true},
			{
				// With the fields left out, drift on the server is still
				// corrected back to the defaults.
				PreConfig: func() {
					target := notifyConfigKey("logger_webhook", name) + " max_retry=7 retry_interval=30s"
					if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), target); err != nil {
						t.Fatalf("drifting %s: %v", name, err)
					}
				},
				Config: testAccMinioLoggerWebhookRetry(name, false),
				Check: testAccCheckServerStoredTarget("logger_webhook", name, map[string]string{
					"max_retry":      "0",
					"retry_interval": "3s",
					"http_timeout":   "5s",
				}),
			},
		},
	})
}

// A bad string value must report the config-value error exactly once per
// field. mergeSchemas adds validateConfigValue to every string field, so
// nesting two mergeSchemas calls (or keeping an explicit validateConfigValue
// on a field that goes through the merge) would report the same error twice.
func TestWebhookRetryStringsValidatedOnce(t *testing.T) {
	bad := `a "b`
	cases := []struct {
		name string
		res  *schema.Resource
		raw  map[string]interface{}
	}{
		{"logger endpoint", resourceMinioLoggerWebhook(), map[string]interface{}{"name": "n", "endpoint": bad}},
		{"logger retry_interval", resourceMinioLoggerWebhook(), map[string]interface{}{"name": "n", "endpoint": "http://l:9/1", "retry_interval": bad}},
		{"audit endpoint", resourceMinioAuditWebhook(), map[string]interface{}{"name": "n", "endpoint": bad}},
		{"audit queue_dir", resourceMinioAuditWebhook(), map[string]interface{}{"name": "n", "endpoint": "http://a:9/1", "queue_dir": bad}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var joined string
			for _, d := range tc.res.Validate(terraform.NewResourceConfigRaw(tc.raw)) {
				joined += d.Summary + " " + d.Detail + " "
			}
			if n := strings.Count(joined, "cannot contain a double quote together with whitespace"); n != 1 {
				t.Errorf("%s: config-value error appeared %d times, want 1; diags: %s", tc.name, n, joined)
			}
		})
	}
}

// The validators mirror what an enabled MinIO target enforces (internal/
// logger/config.go): retry_interval at most 1m, http_timeout at least 1s.
// Values the server accepts stay accepted here, even the odd ones.
func TestWebhookRetryValidators(t *testing.T) {
	intervalCases := []struct {
		value string
		errs  int
	}{
		{"10s", 0},
		{"1m", 0},
		{"0s", 0},
		{"-5s", 0}, // the server only checks the upper bound
		{"1m1s", 1},
		{"2m", 1},
		{"soon", 1},
	}
	for _, tc := range intervalCases {
		_, errs := validateWebhookRetryInterval(tc.value, "retry_interval")
		if len(errs) != tc.errs {
			t.Errorf("retry_interval %q: got %d errors, want %d", tc.value, len(errs), tc.errs)
		}
	}

	timeoutCases := []struct {
		value string
		errs  int
	}{
		{"5s", 0},
		{"1s", 0},
		{"999ms", 1},
		{"500ms", 1},
		{"fast", 1},
	}
	for _, tc := range timeoutCases {
		_, errs := validateWebhookHTTPTimeout(tc.value, "http_timeout")
		if len(errs) != tc.errs {
			t.Errorf("http_timeout %q: got %d errors, want %d", tc.value, len(errs), tc.errs)
		}
	}
}

// MinIO only enforces the limits on an enabled target, so the provider
// rejects them at plan time for both resources.
func TestAccMinioWebhookRetry_planChecks(t *testing.T) {
	cases := []struct {
		name   string
		config string
		expect string
	}{
		{
			name:   "audit negative max_retry",
			config: testAccMinioDisabledTarget("minio_audit_webhook", "tfacc-"+acctest.RandString(6), "  endpoint  = \"http://audit.example.com/ingest\"\n  max_retry = -1"),
			expect: `at least \(0\)`,
		},
		{
			name:   "audit retry_interval above one minute",
			config: testAccMinioDisabledTarget("minio_audit_webhook", "tfacc-"+acctest.RandString(6), "  endpoint       = \"http://audit.example.com/ingest\"\n  retry_interval = \"2m\""),
			expect: `maximum of '1m'`,
		},
		{
			name:   "audit http_timeout below one second",
			config: testAccMinioDisabledTarget("minio_audit_webhook", "tfacc-"+acctest.RandString(6), "  endpoint     = \"http://audit.example.com/ingest\"\n  http_timeout = \"500ms\""),
			expect: `minimum of '1s'`,
		},
		{
			name:   "audit unparseable retry_interval",
			config: testAccMinioDisabledTarget("minio_audit_webhook", "tfacc-"+acctest.RandString(6), "  endpoint       = \"http://audit.example.com/ingest\"\n  retry_interval = \"soon\""),
			expect: `golang duration`,
		},
		{
			name:   "logger negative max_retry",
			config: testAccMinioDisabledTarget("minio_logger_webhook", "tfacc-"+acctest.RandString(6), "  endpoint  = \"http://logger.example.com/logs\"\n  max_retry = -1"),
			expect: `at least \(0\)`,
		},
		{
			name:   "logger retry_interval above one minute",
			config: testAccMinioDisabledTarget("minio_logger_webhook", "tfacc-"+acctest.RandString(6), "  endpoint       = \"http://logger.example.com/logs\"\n  retry_interval = \"90s\""),
			expect: `maximum of '1m'`,
		},
		{
			name:   "logger http_timeout below one second",
			config: testAccMinioDisabledTarget("minio_logger_webhook", "tfacc-"+acctest.RandString(6), "  endpoint     = \"http://logger.example.com/logs\"\n  http_timeout = \"100ms\""),
			expect: `minimum of '1s'`,
		},
		{
			name:   "logger unparseable http_timeout",
			config: testAccMinioDisabledTarget("minio_logger_webhook", "tfacc-"+acctest.RandString(6), "  endpoint     = \"http://logger.example.com/logs\"\n  http_timeout = \"fast\""),
			expect: `golang duration`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:          func() { testAccPreCheck(t) },
				ProviderFactories: testAccProviders,
				Steps: []resource.TestStep{
					{
						Config:      tc.config,
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(tc.expect),
					},
				},
			})
		})
	}
}
