variable "kafka_password" {
  description = "SASL password for the Kafka cluster"
  type        = string
  sensitive   = true
}
