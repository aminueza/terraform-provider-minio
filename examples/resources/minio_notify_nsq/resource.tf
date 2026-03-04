resource "minio_notify_nsq" "events" {
  name         = "primary"
  nsqd_address = "nsqd.example.com:4150"
  topic        = "minio-events"
}
