resource "minio_notify_nats" "events" {
  name    = "primary"
  address = "nats.example.com:4222"
  subject = "minio.events"

  username  = "minio"
  password  = var.nats_password
  jetstream = true
}
