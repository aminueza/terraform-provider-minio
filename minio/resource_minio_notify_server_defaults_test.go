package minio

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// The targets below are disabled, so MinIO stores them without reaching the
// broker or database, and the tests run without those services.

func testAccMinioNotifyDisabledTarget(kind, name, fields string) string {
	return fmt.Sprintf(`
resource "minio_notify_%s" "test" {
  name   = %q
  enable = false
%s
}
`, kind, name, fields)
}

// MinIO seeds subsystem defaults into every target it stores. A default the
// configuration never set must not show up as a diff once the server has
// restarted and returns it. See issue #1307.
func testAccMinioNotifySeededDefaultsCase(t *testing.T, kind, fields string, seeded map[string]string) {
	resourceName := "minio_notify_" + kind + ".test"
	name := "tfacc-" + acctest.RandString(6)
	config := testAccMinioNotifyDisabledTarget(kind, name, fields)

	checks := []resource.TestCheckFunc{testAccCheckServerStoredTarget("notify_"+kind, name, seeded)}
	for key, value := range seeded {
		checks = append(checks, resource.TestCheckResourceAttr(resourceName, key, value))
	}

	// Sequential on purpose: the server checks restart the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("notify_"+kind, name),
		Steps: []resource.TestStep{
			{Config: config},
			{
				Config: config,
				Check:  testAccCheckServerStoredTarget("notify_"+kind, name, seeded),
			},
			{
				Config: config,
				Check:  resource.ComposeTestCheckFunc(checks...),
			},
			{Config: config, PlanOnly: true},
		},
	})
}

func TestAccMinioNotifyKafka_seededDefaults(t *testing.T) {
	testAccMinioNotifySeededDefaultsCase(t, "kafka", `  brokers = "localhost:9092"
  topic   = "events"`, map[string]string{"sasl_mechanism": "plain"})
}

func TestAccMinioNotifyMqtt_seededDefaults(t *testing.T) {
	testAccMinioNotifySeededDefaultsCase(t, "mqtt", `  broker = "tcp://localhost:1883"
  topic  = "events"`, map[string]string{"keep_alive_interval": "0s", "reconnect_interval": "0s"})
}

func TestAccMinioNotifyNats_seededDefaults(t *testing.T) {
	testAccMinioNotifySeededDefaultsCase(t, "nats", `  address = "localhost:4222"
  subject = "events"`, map[string]string{"ping_interval": "0"})
}

// The DSN must reach the server as dsn_string, the key notify_mysql reads; a
// connection_string key is dropped and leaves the target without a DSN.
func TestAccMinioNotifyMysql_dsnStored(t *testing.T) {
	resourceName := "minio_notify_mysql.test"
	name := "tfacc-" + acctest.RandString(6)
	withDSN := func(dsn string) string {
		return testAccMinioNotifyDisabledTarget("mysql", name, fmt.Sprintf(`  connection_string = %q
  table             = "events"
  format            = "namespace"`, dsn))
	}

	// Sequential on purpose: the server checks restart the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("notify_mysql", name),
		Steps: []resource.TestStep{
			{
				Config: withDSN("user:pass@tcp(localhost:3306)/events"),
				Check: testAccCheckServerStoredTarget("notify_mysql", name, map[string]string{
					"dsn_string": "user:pass@tcp(localhost:3306)/events",
				}),
			},
			{
				Config: withDSN("user:pass@tcp(localhost:3306)/events"),
				Check:  resource.TestCheckResourceAttr(resourceName, "connection_string", "user:pass@tcp(localhost:3306)/events"),
			},
			{
				Config: withDSN("other:secret@tcp(db.internal:3306)/audit"),
				Check: testAccCheckServerStoredTarget("notify_mysql", name, map[string]string{
					"dsn_string": "other:secret@tcp(db.internal:3306)/audit",
				}),
			},
			{Config: withDSN("other:secret@tcp(db.internal:3306)/audit"), PlanOnly: true},
		},
	})
}

func TestBuildNotifyMysqlCfgWritesDsnString(t *testing.T) {
	d := schema.TestResourceDataRaw(t, resourceMinioNotifyMysql().Schema, map[string]interface{}{
		"name":              "events",
		"connection_string": "user:pass@tcp(localhost:3306)/events",
		"table":             "events",
		"format":            "namespace",
	})

	got := buildNotifyMysqlCfg(d, nil)

	if !strings.Contains(got, "dsn_string=user:pass@tcp(localhost:3306)/events") {
		t.Errorf("config %q does not carry the DSN as dsn_string", got)
	}
	if strings.Contains(got, "connection_string=") {
		t.Errorf("config %q still sends connection_string, which notify_mysql drops", got)
	}
}

// A secret the server returns in clear, such as the MySQL DSN read from
// dsn_string, must stay hidden in a data source's plan output too.
func TestNotifyDataSourcesKeepSecretsSensitive(t *testing.T) {
	cases := []struct {
		dataSource string
		resource   *schema.Resource
		field      string
	}{
		{"minio_notify_mysql", dataSourceMinioNotifyMysql(), "connection_string"},
		{"minio_notify_postgres", dataSourceMinioNotifyPostgres(), "connection_string"},
		{"minio_notify_amqp", dataSourceMinioNotifyAmqp(), "url"},
		{"minio_notify_elasticsearch", dataSourceMinioNotifyElasticsearch(), "url"},
		{"minio_notify_kafka", dataSourceMinioNotifyKafka(), "sasl_password"},
		{"minio_notify_nats", dataSourceMinioNotifyNats(), "token"},
		{"minio_notify_webhook", dataSourceMinioNotifyWebhook(), "auth_token"},
	}

	for _, tc := range cases {
		t.Run(tc.dataSource+"."+tc.field, func(t *testing.T) {
			s, ok := tc.resource.Schema[tc.field]
			if !ok {
				t.Fatalf("%s has no %q attribute", tc.dataSource, tc.field)
			}
			if !s.Sensitive {
				t.Errorf("%s.%s is not sensitive, so its value shows in clear in plan output", tc.dataSource, tc.field)
			}
			if !s.Computed || s.Optional || s.Required {
				t.Errorf("%s.%s must stay computed-only in a data source", tc.dataSource, tc.field)
			}
		})
	}
}

// A credential-carrying value changed on the server outside Terraform must
// show up in the plan, and the next apply must write the configured value
// back. The server returns these keys in clear, so the read can compare
// them. See issue #1328.
func testAccMinioNotifyCredentialDriftCase(t *testing.T, kind, key, fields, configured, outside string) {
	resourceName := "minio_notify_" + kind + ".test"
	name := "tfacc-" + acctest.RandString(6)
	config := testAccMinioNotifyDisabledTarget(kind, name, fmt.Sprintf(fields, configured))

	// Sequential on purpose: the server checks restart the primary test
	// server, which must not be running underneath other tests.
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckServerRemovedTarget("notify_"+kind, name),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  testAccCheckServerStoredTarget("notify_"+kind, name, map[string]string{key: configured}),
			},
			{
				PreConfig: func() {
					// enable=off keeps the target disabled: a write without it
					// enables the target, and MinIO then dials the endpoint.
					changed := notifyConfigKey("notify_"+kind, name) + " " + buildConfigParam(key, outside) + " enable=off"
					if _, err := testAccClient().S3Admin.SetConfigKV(context.Background(), changed); err != nil {
						t.Fatalf("changing %s of %s outside Terraform: %v", key, name, err)
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
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, key, configured),
					testAccCheckServerStoredTarget("notify_"+kind, name, map[string]string{key: configured}),
				),
			},
			{Config: config, PlanOnly: true},
		},
	})
}

func TestAccMinioNotifyAmqp_urlChangedOutsideTerraform(t *testing.T) {
	testAccMinioNotifyCredentialDriftCase(t, "amqp", "url", `  url = %q`,
		"amqp://user:pw1@localhost:5672", "amqp://other:pw2@broker.internal:5672")
}

func TestAccMinioNotifyElasticsearch_urlChangedOutsideTerraform(t *testing.T) {
	testAccMinioNotifyCredentialDriftCase(t, "elasticsearch", "url", `  url    = %q
  index  = "minio-events"
  format = "namespace"`,
		"http://user:pw1@localhost:9200", "http://other:pw2@search.internal:9200")
}

func TestAccMinioNotifyPostgres_connectionStringChangedOutsideTerraform(t *testing.T) {
	testAccMinioNotifyCredentialDriftCase(t, "postgres", "connection_string", `  connection_string = %q
  table             = "events"
  format            = "namespace"`,
		"host=localhost port=5432 dbname=minio user=minio password=pw1 sslmode=disable",
		"host=db.internal port=5432 dbname=minio user=other password=pw2 sslmode=disable")
}

func TestReadNotifyCredentialFields(t *testing.T) {
	cases := []struct {
		name     string
		resource *schema.Resource
		read     func(map[string]string, *schema.ResourceData) diag.Diagnostics
		fields   map[string]interface{}
		key      string
		server   string
	}{
		{
			name:     "amqp url",
			resource: resourceMinioNotifyAmqp(),
			read:     readNotifyAmqpFields,
			fields:   map[string]interface{}{"url": "amqp://user:pw1@localhost:5672"},
			key:      "url",
			server:   "amqp://other:pw2@broker.internal:5672",
		},
		{
			name:     "elasticsearch url",
			resource: resourceMinioNotifyElasticsearch(),
			read:     readNotifyElasticsearchFields,
			fields:   map[string]interface{}{"url": "http://localhost:9200", "index": "minio-events", "format": "namespace"},
			key:      "url",
			server:   "http://other:pw2@search.internal:9200",
		},
		{
			name:     "postgres connection_string",
			resource: resourceMinioNotifyPostgres(),
			read:     readNotifyPostgresFields,
			fields:   map[string]interface{}{"connection_string": "host=localhost password=pw1", "table": "events", "format": "namespace"},
			key:      "connection_string",
			server:   "host=db.internal port=5432 user=other password=pw2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := map[string]interface{}{"name": "events"}
			for k, v := range tc.fields {
				raw[k] = v
			}
			d := schema.TestResourceDataRaw(t, tc.resource.Schema, raw)

			if diags := tc.read(map[string]string{tc.key: tc.server}, d); diags.HasError() {
				t.Fatalf("read failed: %v", diags)
			}

			if got := d.Get(tc.key).(string); got != tc.server {
				t.Errorf("%s = %q after the read, want the server's %q", tc.key, got, tc.server)
			}
		})
	}
}
