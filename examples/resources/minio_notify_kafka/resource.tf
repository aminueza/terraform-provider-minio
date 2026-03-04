resource "minio_notify_kafka" "events" {
  name    = "primary"
  brokers = "kafka1:9092,kafka2:9092,kafka3:9092"
  topic   = "minio-bucket-events"

  sasl_username  = "minio"
  sasl_password  = var.kafka_password
  sasl_mechanism = "plain"
  tls            = true
}
