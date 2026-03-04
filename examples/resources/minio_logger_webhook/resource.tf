resource "minio_logger_webhook" "splunk" {
  name       = "splunk"
  endpoint   = "https://splunk.example.com:8088/services/collector"
  auth_token = var.splunk_token
  batch_size = 100
}
