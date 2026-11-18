package minio

import (
	"fmt"
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
