data "minio_config_history" "recent" {
  count = 10
}