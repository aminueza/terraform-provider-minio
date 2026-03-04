resource "minio_server_config_storage_class" "main" {
  standard = "EC:4"
  rrs      = "EC:2"
}
