resource "minio_server_config_api" "main" {
  stale_uploads_expiry             = "12h"
  stale_uploads_cleanup_interval   = "6h"
  cors_allow_origin                = "https://app.example.com,https://admin.example.com"
  transition_workers               = "200"
  root_access                      = true
}
