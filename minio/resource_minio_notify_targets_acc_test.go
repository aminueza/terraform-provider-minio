package minio

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// All notify target acceptance tests use enable=false so the MinIO server
// stores the configuration without attempting to connect to the target service.
// This allows the tests to run in the standard CI environment without requiring
// external message brokers or databases.

func TestAccMinioNotifyAmqp_basic(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_amqp.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyAmqpConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "url", "amqp://guest:guest@localhost:5672"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
			{
				Config: testAccMinioNotifyAmqpConfigUpdate(name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "exchange", "my-exchange"),
					resource.TestCheckResourceAttr(resourceName, "routing_key", "events"),
				),
			},
		},
	})
}

func TestAccMinioNotifyKafka_basic(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_kafka.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyKafkaConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "brokers", "localhost:9092"),
					resource.TestCheckResourceAttr(resourceName, "topic", "minio-events"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func TestAccMinioNotifyMqtt_basic(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_mqtt.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyMqttConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "broker", "tcp://localhost:1883"),
					resource.TestCheckResourceAttr(resourceName, "topic", "minio/events"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func TestAccMinioNotifyNats_basic(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_nats.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyNatsConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "address", "localhost:4222"),
					resource.TestCheckResourceAttr(resourceName, "subject", "minio-events"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func TestAccMinioNotifyNsq_basic(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_nsq.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyNsqConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "nsqd_address", "localhost:4150"),
					resource.TestCheckResourceAttr(resourceName, "topic", "minio-events"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func TestAccMinioNotifyMysql_basic(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_mysql.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyMysqlConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "table", "minio_events"),
					resource.TestCheckResourceAttr(resourceName, "format", "namespace"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func TestAccMinioNotifyPostgres_basic(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_postgres.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyPostgresConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "table", "minio_events"),
					resource.TestCheckResourceAttr(resourceName, "format", "namespace"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func TestAccMinioNotifyElasticsearch_basic(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_elasticsearch.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyElasticsearchConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "url", "http://localhost:9200"),
					resource.TestCheckResourceAttr(resourceName, "index", "minio-events"),
					resource.TestCheckResourceAttr(resourceName, "format", "namespace"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func TestAccMinioNotifyRedis_basic(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_redis.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyRedisConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "address", "localhost:6379"),
					resource.TestCheckResourceAttr(resourceName, "key", "minio-events"),
					resource.TestCheckResourceAttr(resourceName, "format", "namespace"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func TestAccMinioLoggerWebhook_basic(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_logger_webhook.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioLoggerWebhookConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "endpoint", "http://log-collector:8080/logs"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
			{
				Config: testAccMinioLoggerWebhookConfigUpdate(name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "batch_size", "100"),
				),
			},
		},
	})
}

func TestAccMinioAuditKafka_basic(t *testing.T) {
	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_audit_kafka.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioAuditKafkaConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "brokers", "localhost:9092"),
					resource.TestCheckResourceAttr(resourceName, "topic", "minio-audit"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func testAccMinioNotifyAmqpConfig(name string) string {
	return fmt.Sprintf(`
resource "minio_notify_amqp" "test" {
  name   = %[1]q
  url    = "amqp://guest:guest@localhost:5672"
  enable = false
}
`, name)
}

func testAccMinioNotifyAmqpConfigUpdate(name string) string {
	return fmt.Sprintf(`
resource "minio_notify_amqp" "test" {
  name        = %[1]q
  url         = "amqp://guest:guest@localhost:5672"
  exchange    = "my-exchange"
  routing_key = "events"
  enable      = false
}
`, name)
}

func testAccMinioNotifyKafkaConfig(name string) string {
	return fmt.Sprintf(`
resource "minio_notify_kafka" "test" {
  name    = %[1]q
  brokers = "localhost:9092"
  topic   = "minio-events"
  enable  = false
}
`, name)
}

func testAccMinioNotifyMqttConfig(name string) string {
	return fmt.Sprintf(`
resource "minio_notify_mqtt" "test" {
  name   = %[1]q
  broker = "tcp://localhost:1883"
  topic  = "minio/events"
  enable = false
}
`, name)
}

func testAccMinioNotifyNatsConfig(name string) string {
	return fmt.Sprintf(`
resource "minio_notify_nats" "test" {
  name    = %[1]q
  address = "localhost:4222"
  subject = "minio-events"
  enable  = false
}
`, name)
}

func testAccMinioNotifyNsqConfig(name string) string {
	return fmt.Sprintf(`
resource "minio_notify_nsq" "test" {
  name         = %[1]q
  nsqd_address = "localhost:4150"
  topic        = "minio-events"
  enable       = false
}
`, name)
}

func testAccMinioNotifyMysqlConfig(name string) string {
	return fmt.Sprintf(`
resource "minio_notify_mysql" "test" {
  name              = %[1]q
  connection_string = "root:password@tcp(localhost:3306)/minio"
  table             = "minio_events"
  format            = "namespace"
  enable            = false
}
`, name)
}

func testAccMinioNotifyPostgresConfig(name string) string {
	return fmt.Sprintf(`
resource "minio_notify_postgres" "test" {
  name              = %[1]q
  connection_string = "postgres://postgres:password@localhost:5432/minio?sslmode=disable"
  table             = "minio_events"
  format            = "namespace"
  enable            = false
}
`, name)
}

func testAccMinioNotifyElasticsearchConfig(name string) string {
	return fmt.Sprintf(`
resource "minio_notify_elasticsearch" "test" {
  name   = %[1]q
  url    = "http://localhost:9200"
  index  = "minio-events"
  format = "namespace"
  enable = false
}
`, name)
}

func testAccMinioNotifyRedisConfig(name string) string {
	return fmt.Sprintf(`
resource "minio_notify_redis" "test" {
  name    = %[1]q
  address = "localhost:6379"
  key     = "minio-events"
  format  = "namespace"
  enable  = false
}
`, name)
}

func testAccMinioLoggerWebhookConfig(name string) string {
	return fmt.Sprintf(`
resource "minio_logger_webhook" "test" {
  name     = %[1]q
  endpoint = "http://log-collector:8080/logs"
  enable   = false
}
`, name)
}

func testAccMinioLoggerWebhookConfigUpdate(name string) string {
	return fmt.Sprintf(`
resource "minio_logger_webhook" "test" {
  name       = %[1]q
  endpoint   = "http://log-collector:8080/logs"
  batch_size = 100
  enable     = false
}
`, name)
}

func testAccMinioAuditKafkaConfig(name string) string {
	return fmt.Sprintf(`
resource "minio_audit_kafka" "test" {
  name    = %[1]q
  brokers = "localhost:9092"
  topic   = "minio-audit"
  enable  = false
}
`, name)
}
