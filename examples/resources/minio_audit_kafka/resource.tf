resource "minio_audit_kafka" "compliance" {
  name           = "compliance"
  brokers        = "kafka1:9092,kafka2:9092"
  topic          = "minio-audit"
  sasl           = true
  sasl_username  = "minio"
  sasl_password  = var.kafka_password
  sasl_mechanism = "plain"
  tls            = true
}
