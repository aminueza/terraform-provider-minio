data "minio_notify_kafka" "example" {
  name = "my-kafka"
}

output "kafka_brokers" {
  value = data.minio_notify_kafka.example.brokers
}
