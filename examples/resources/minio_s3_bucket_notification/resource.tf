# Configure bucket notifications for events
resource "minio_s3_bucket_notification" "example" {
  bucket = "my-bucket"

  queue {
    id     = "notification-queue"
    queue_arn = "arn:minio:sqs::primary:webhook"
    events = ["s3:ObjectCreated:*"]
    filter_prefix = "uploads/"
    filter_suffix = ".jpg"
  }
}
