data "minio_notify_amqp" "example" {
  name = "my-amqp"
}

output "amqp_exchange" {
  value = data.minio_notify_amqp.example.exchange
}
