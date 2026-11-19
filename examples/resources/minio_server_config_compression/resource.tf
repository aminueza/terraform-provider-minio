resource "minio_server_config_compression" "main" {
  enable             = true
  allow_encryption   = false
  extensions         = [".txt", ".log", ".csv"]
  mime_types         = ["text/*", "application/json"]
  comment            = "Compress text objects"
}
