resource "minio_notify_amqp" "events" {
  name          = "rabbitmq"
  url           = "amqp://user:password@rabbitmq.example.com:5672"
  exchange      = "minio-events"
  exchange_type = "fanout"
  routing_key   = "bucketevents"
  durable       = true
}
