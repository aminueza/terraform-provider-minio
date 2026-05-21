package minio

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

// All broker-based notify target acceptance tests are guarded by an environment
// variable so they can be skipped in CI environments where the external service
// is not available.
//
// HTTP-based targets (logger_webhook) do not establish a persistent connection
// on config apply and therefore work with any URL when enable=false.

func TestAccMinioNotifyAmqp_basic(t *testing.T) {
	amqpURL := os.Getenv("TF_ACC_AMQP_URL")
	if amqpURL == "" {
		t.Skip("TF_ACC_AMQP_URL not set — skipping AMQP notify target acceptance test")
	}

	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_amqp.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyAmqpConfig(name, amqpURL),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
			{
				Config: testAccMinioNotifyAmqpConfigUpdate(name, amqpURL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "exchange", "my-exchange"),
					resource.TestCheckResourceAttr(resourceName, "routing_key", "events"),
				),
			},
		},
	})
}

func TestAccMinioNotifyKafka_basic(t *testing.T) {
	brokers := os.Getenv("TF_ACC_KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("TF_ACC_KAFKA_BROKERS not set — skipping Kafka notify target acceptance test")
	}

	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_kafka.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyKafkaConfig(name, brokers),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "brokers", brokers),
					resource.TestCheckResourceAttr(resourceName, "topic", "minio-events"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func TestAccMinioNotifyMqtt_basic(t *testing.T) {
	broker := os.Getenv("TF_ACC_MQTT_BROKER")
	if broker == "" {
		t.Skip("TF_ACC_MQTT_BROKER not set — skipping MQTT notify target acceptance test")
	}

	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_mqtt.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyMqttConfig(name, broker),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "broker", broker),
					resource.TestCheckResourceAttr(resourceName, "topic", "minio/events"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func TestAccMinioNotifyNats_basic(t *testing.T) {
	address := os.Getenv("TF_ACC_NATS_ADDRESS")
	if address == "" {
		t.Skip("TF_ACC_NATS_ADDRESS not set — skipping NATS notify target acceptance test")
	}

	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_nats.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyNatsConfig(name, address),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "address", address),
					resource.TestCheckResourceAttr(resourceName, "subject", "minio-events"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func TestAccMinioNotifyNsq_basic(t *testing.T) {
	nsqdAddress := os.Getenv("TF_ACC_NSQ_ADDRESS")
	if nsqdAddress == "" {
		t.Skip("TF_ACC_NSQ_ADDRESS not set — skipping NSQ notify target acceptance test")
	}

	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_nsq.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyNsqConfig(name, nsqdAddress),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "nsqd_address", nsqdAddress),
					resource.TestCheckResourceAttr(resourceName, "topic", "minio-events"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func TestAccMinioNotifyMysql_basic(t *testing.T) {
	dsn := os.Getenv("TF_ACC_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TF_ACC_MYSQL_DSN not set — skipping MySQL notify target acceptance test")
	}

	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_mysql.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyMysqlConfig(name, dsn),
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
	dsn := os.Getenv("TF_ACC_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TF_ACC_POSTGRES_DSN not set — skipping PostgreSQL notify target acceptance test")
	}

	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_postgres.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyPostgresConfig(name, dsn),
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
	esURL := os.Getenv("TF_ACC_ELASTICSEARCH_URL")
	if esURL == "" {
		t.Skip("TF_ACC_ELASTICSEARCH_URL not set — skipping Elasticsearch notify target acceptance test")
	}

	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_elasticsearch.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyElasticsearchConfig(name, esURL),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "url", esURL),
					resource.TestCheckResourceAttr(resourceName, "index", "minio-events"),
					resource.TestCheckResourceAttr(resourceName, "format", "namespace"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func TestAccMinioNotifyRedis_basic(t *testing.T) {
	address := os.Getenv("TF_ACC_REDIS_ADDRESS")
	if address == "" {
		t.Skip("TF_ACC_REDIS_ADDRESS not set — skipping Redis notify target acceptance test")
	}

	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_notify_redis.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioNotifyRedisConfig(name, address),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "address", address),
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
					resource.TestCheckResourceAttr(resourceName, "endpoint", "http://log-collector.example.com/logs"),
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
	brokers := os.Getenv("TF_ACC_KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("TF_ACC_KAFKA_BROKERS not set — skipping audit_kafka acceptance test")
	}

	name := "tfacc-" + acctest.RandString(6)
	resourceName := "minio_audit_kafka.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckNotifyTargetDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccMinioAuditKafkaConfig(name, brokers),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNotifyTargetExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "brokers", brokers),
					resource.TestCheckResourceAttr(resourceName, "topic", "minio-audit"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
				),
			},
		},
	})
}

func testAccMinioNotifyAmqpConfig(name, amqpURL string) string {
	return fmt.Sprintf(`
resource "minio_notify_amqp" "test" {
  name   = %[1]q
  url    = %[2]q
  enable = false
}
`, name, amqpURL)
}

func testAccMinioNotifyAmqpConfigUpdate(name, amqpURL string) string {
	return fmt.Sprintf(`
resource "minio_notify_amqp" "test" {
  name        = %[1]q
  url         = %[2]q
  exchange    = "my-exchange"
  routing_key = "events"
  enable      = false
}
`, name, amqpURL)
}

func testAccMinioNotifyKafkaConfig(name, brokers string) string {
	return fmt.Sprintf(`
resource "minio_notify_kafka" "test" {
  name    = %[1]q
  brokers = %[2]q
  topic   = "minio-events"
  enable  = false
}
`, name, brokers)
}

func testAccMinioNotifyMqttConfig(name, broker string) string {
	return fmt.Sprintf(`
resource "minio_notify_mqtt" "test" {
  name   = %[1]q
  broker = %[2]q
  topic  = "minio/events"
  enable = false
}
`, name, broker)
}

func testAccMinioNotifyNatsConfig(name, address string) string {
	return fmt.Sprintf(`
resource "minio_notify_nats" "test" {
  name    = %[1]q
  address = %[2]q
  subject = "minio-events"
  enable  = false
}
`, name, address)
}

func testAccMinioNotifyNsqConfig(name, nsqdAddress string) string {
	return fmt.Sprintf(`
resource "minio_notify_nsq" "test" {
  name         = %[1]q
  nsqd_address = %[2]q
  topic        = "minio-events"
  enable       = false
}
`, name, nsqdAddress)
}

func testAccMinioNotifyMysqlConfig(name, dsn string) string {
	return fmt.Sprintf(`
resource "minio_notify_mysql" "test" {
  name              = %[1]q
  connection_string = %[2]q
  table             = "minio_events"
  format            = "namespace"
  enable            = false
}
`, name, dsn)
}

func testAccMinioNotifyPostgresConfig(name, dsn string) string {
	return fmt.Sprintf(`
resource "minio_notify_postgres" "test" {
  name              = %[1]q
  connection_string = %[2]q
  table             = "minio_events"
  format            = "namespace"
  enable            = false
}
`, name, dsn)
}

func testAccMinioNotifyElasticsearchConfig(name, esURL string) string {
	return fmt.Sprintf(`
resource "minio_notify_elasticsearch" "test" {
  name   = %[1]q
  url    = %[2]q
  index  = "minio-events"
  format = "namespace"
  enable = false
}
`, name, esURL)
}

func testAccMinioNotifyRedisConfig(name, address string) string {
	return fmt.Sprintf(`
resource "minio_notify_redis" "test" {
  name    = %[1]q
  address = %[2]q
  key     = "minio-events"
  format  = "namespace"
  enable  = false
}
`, name, address)
}

func testAccMinioLoggerWebhookConfig(name string) string {
	return fmt.Sprintf(`
resource "minio_logger_webhook" "test" {
  name     = %[1]q
  endpoint = "http://log-collector.example.com/logs"
  enable   = false
}
`, name)
}

func testAccMinioLoggerWebhookConfigUpdate(name string) string {
	return fmt.Sprintf(`
resource "minio_logger_webhook" "test" {
  name       = %[1]q
  endpoint   = "http://log-collector.example.com/logs"
  batch_size = 100
  enable     = false
}
`, name)
}

func testAccMinioAuditKafkaConfig(name, brokers string) string {
	return fmt.Sprintf(`
resource "minio_audit_kafka" "test" {
  name    = %[1]q
  brokers = %[2]q
  topic   = "minio-audit"
  enable  = false
}
`, name, brokers)
}
