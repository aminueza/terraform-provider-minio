package minio

import (
	"fmt"
	"strings"
	"testing"

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
