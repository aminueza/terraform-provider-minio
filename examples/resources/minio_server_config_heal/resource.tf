resource "minio_server_config_heal" "main" {
  bitrotscan = "on"
  max_sleep  = "250ms"
  max_io     = "100"
}
