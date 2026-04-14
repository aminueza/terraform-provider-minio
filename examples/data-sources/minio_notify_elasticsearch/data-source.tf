data "minio_notify_elasticsearch" "example" {
  name = "my-elasticsearch"
}

output "elasticsearch_index" {
  value = data.minio_notify_elasticsearch.example.index
}
