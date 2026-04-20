data "minio_kms_status" "current" {}

output "kms_reachable" {
  value = data.minio_kms_status.current.state[0].key_store_reachable
}

output "kms_default_key" {
  value = data.minio_kms_status.current.default_key_id
}
