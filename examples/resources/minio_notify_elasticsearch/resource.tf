resource "minio_notify_elasticsearch" "events" {
  name     = "primary"
  url      = "https://elasticsearch.example.com:9200"
  index    = "minio-events"
  format   = "namespace"
  username = "elastic"
  password = var.es_password
}
