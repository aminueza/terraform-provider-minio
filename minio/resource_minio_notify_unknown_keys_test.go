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
)

// MinIO accepts a key its subsystem does not have and glues it onto the
// value written before it. These tests check that the neighbouring values
// reach the server intact. The targets are disabled, so MinIO stores them
// without reaching a broker or endpoint.

func testAccMinioDisabledTarget(resourceType, name, fields string) string {
	return fmt.Sprintf(`
resource "%s" "test" {
  name   = %q
  enable = false
%s
}
`, resourceType, name, fields)
}

func testAccMinioUnknownKeyCase(t *testing.T, resourceType, subsystem, fields string, stored map[string]string) {
	name := "tfacc-" + acctest.RandString(6)
	config := testAccMinioDisabledTarget(resourceType, name, fields)

	// Sequential on purpose: the server check restarts the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget(subsystem, name),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  testAccCheckServerStoredTarget(subsystem, name, stored),
			},
			{Config: config, PlanOnly: true},
		},
	})
}

func TestAccMinioLoggerWebhook_queueSize(t *testing.T) {
	testAccMinioUnknownKeyCase(t, "minio_logger_webhook", "logger_webhook", `  endpoint   = "http://localhost:9999/logs"
  comment    = "primary"
  queue_size = 5000`, map[string]string{
		"comment":    "primary",
		"queue_dir":  "",
		"queue_size": "5000",
	})
}

func TestAccMinioLoggerWebhook_deprecatedQueueLimit(t *testing.T) {
	testAccMinioUnknownKeyCase(t, "minio_logger_webhook", "logger_webhook", `  endpoint    = "http://localhost:9999/logs"
  comment     = "primary"
  queue_limit = 3000`, map[string]string{
		"comment":    "primary",
		"queue_dir":  "",
		"queue_size": "3000",
	})
}

// A logger webhook written by an older provider with batch_size and
// queue_limit was stored with queue_limit glued onto batch_size and the
// default queue_size. The plan must show that drift, and the next apply must
// write both keys again.
func TestAccMinioLoggerWebhook_queueLimitCorruptedByOlderProvider(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	config := testAccMinioDisabledTarget("minio_logger_webhook", name, `  endpoint    = "http://localhost:9999/logs"
  batch_size  = 10
  queue_limit = 3000`)

	// Sequential on purpose: the server checks restart the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("logger_webhook", name),
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig: func() {
					old := "logger_webhook:" + name + " endpoint=http://localhost:9999/logs queue_size=100000 batch_size=10 queue_limit=3000 enable=off"
					if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), old); err != nil {
						t.Fatalf("writing %s the way the older provider did: %v", name, err)
					}
					if err := testAccRestartPrimaryServer(); err != nil {
						t.Fatalf("restarting the server: %v", err)
					}
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				Check: testAccCheckServerStoredTarget("logger_webhook", name, map[string]string{
					"batch_size": "10",
					"queue_size": "3000",
				}),
			},
			{Config: config, PlanOnly: true},
		},
	})
}

func TestAccMinioLoggerWebhook_queueSizeConflictsWithQueueLimit(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioDisabledTarget("minio_logger_webhook", "tfacc-"+acctest.RandString(6), `  endpoint    = "http://localhost:9999/logs"
  queue_size  = 5000
  queue_limit = 3000`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`conflicts with`),
			},
		},
	})
}

func TestAccMinioAuditKafka_queueSize(t *testing.T) {
	testAccMinioUnknownKeyCase(t, "minio_audit_kafka", "audit_kafka", `  brokers    = "localhost:9092"
  topic      = "audit"
  queue_size = 5000`, map[string]string{
		"topic":      "audit",
		"queue_size": "5000",
	})
}

func TestAccMinioNotifyNats_userCredentialsNotSent(t *testing.T) {
	testAccMinioUnknownKeyCase(t, "minio_notify_nats", "notify_nats", `  address          = "localhost:4222"
  subject          = "events"
  ping_interval    = "0"
  user_credentials = "/etc/nats/user.creds"`, map[string]string{
		"subject": "events",
	})
}

func TestNotifyBuildersSendOnlyKnownKeys(t *testing.T) {
	cases := []struct {
		name      string
		resource  *schema.Resource
		build     func(*schema.ResourceData, interface{}) string
		raw       map[string]interface{}
		mustHave  []string
		mustNotBe []string
	}{
		{
			name:      "logger_webhook sends queue_size",
			resource:  resourceMinioLoggerWebhook(),
			build:     buildLoggerWebhookCfg,
			raw:       map[string]interface{}{"name": "n", "endpoint": "http://localhost:9999/logs", "queue_size": 5000},
			mustHave:  []string{"queue_size=5000"},
			mustNotBe: []string{"queue_limit=", "max_retry=", "retry_interval=", "http_timeout="},
		},
		{
			name:      "logger_webhook sends retry keys when set",
			resource:  resourceMinioLoggerWebhook(),
			build:     buildLoggerWebhookCfg,
			raw:       map[string]interface{}{"name": "n", "endpoint": "http://localhost:9999/logs", "max_retry": 3, "retry_interval": "10s", "http_timeout": "10s"},
			mustHave:  []string{"max_retry=3", "retry_interval=10s", "http_timeout=10s"},
			mustNotBe: []string{"queue_limit="},
		},
		{
			name:      "audit_kafka sends queue_size",
			resource:  resourceMinioAuditKafka(),
			build:     buildAuditKafkaCfg,
			raw:       map[string]interface{}{"name": "n", "brokers": "localhost:9092", "topic": "audit", "queue_size": 5000},
			mustHave:  []string{"queue_size=5000", "topic=audit"},
			mustNotBe: []string{"queue_limit="},
		},
		{
			name:      "notify_nats drops user_credentials",
			resource:  resourceMinioNotifyNats(),
			build:     buildNotifyNatsCfg,
			raw:       map[string]interface{}{"name": "n", "address": "localhost:4222", "subject": "events", "user_credentials": "/etc/nats/user.creds"},
			mustHave:  []string{"subject=events"},
			mustNotBe: []string{"user_credentials="},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.build(schema.TestResourceDataRaw(t, tc.resource.Schema, tc.raw), nil)
			for _, want := range tc.mustHave {
				if !strings.Contains(got, want) {
					t.Errorf("config %q does not contain %q", got, want)
				}
			}
			for _, bad := range tc.mustNotBe {
				if strings.Contains(got, bad) {
					t.Errorf("config %q contains %q, a key the subsystem does not have", got, bad)
				}
			}
		})
	}
}
