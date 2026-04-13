data "minio_notify_webhook" "example" {
  name = "my-webhook"
}

output "webhook_endpoint" {
  value = data.minio_notify_webhook.example.endpoint
}
