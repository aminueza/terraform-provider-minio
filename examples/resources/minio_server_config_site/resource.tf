resource "minio_server_config_site" "main" {
  name    = "dc1-rack2"
  region  = "us-east-1"
  comment = "Primary datacenter"
}
