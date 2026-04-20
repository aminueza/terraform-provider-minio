data "minio_kms_metrics" "current" {}

output "kms_requests_ok" {
  value = data.minio_kms_metrics.current.request_ok
}

output "kms_uptime_seconds" {
  value = data.minio_kms_metrics.current.uptime_seconds
}
