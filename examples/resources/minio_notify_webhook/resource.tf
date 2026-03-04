# Primary webhook target for bucket event notifications
resource "minio_notify_webhook" "primary" {
  name       = "primary"
  endpoint   = "https://webhook.example.com/minio-events"
  auth_token = var.webhook_token
  queue_limit = 100000
}

# Backup target with persistent queue for offline resilience
resource "minio_notify_webhook" "backup" {
  name       = "backup"
  endpoint   = "https://backup.example.com/events"
  queue_dir  = "/opt/minio/events/backup"
  comment    = "Backup webhook with persistent queue"
}

# Use the target in a bucket notification
resource "minio_s3_bucket_notification" "events" {
  bucket = "my-bucket"

  queue {
    queue_arn = "arn:minio:sqs::primary:webhook"
    events    = ["s3:ObjectCreated:*", "s3:ObjectRemoved:*"]
  }

  depends_on = [minio_notify_webhook.primary]
}
