resource "minio_notify_redis" "events" {
  name     = "primary"
  address  = "redis.example.com:6379"
  key      = "minio-events"
  format   = "namespace"
  password = var.redis_password
}
