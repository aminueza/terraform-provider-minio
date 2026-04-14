data "minio_notify_nats" "example" {
  name = "my-nats"
}

output "nats_subject" {
  value = data.minio_notify_nats.example.subject
}
