# Send audit events to Splunk HEC
resource "minio_audit_webhook" "splunk" {
  name       = "splunk"
  endpoint   = "https://splunk.example.com:8088/services/collector"
  auth_token = var.splunk_hec_token
  queue_size = 100000
  batch_size = 500
}

# Send audit events to a secondary target (disabled initially)
resource "minio_audit_webhook" "backup" {
  name     = "backup"
  endpoint = "https://audit-backup.example.com/ingest"
  enable   = false
}
