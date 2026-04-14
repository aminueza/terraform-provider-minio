data "minio_notify_redis" "example" {
  name = "my-redis"
}

output "redis_key" {
  value = data.minio_notify_redis.example.key
}
