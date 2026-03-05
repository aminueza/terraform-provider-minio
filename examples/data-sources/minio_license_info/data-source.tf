data "minio_license_info" "current" {}

output "license_plan" {
  value = data.minio_license_info.current.plan
}
