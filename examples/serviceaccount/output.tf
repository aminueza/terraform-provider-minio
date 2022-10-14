 output "minio_access_key" {
  value = minio_iam_service_account.test_service_account.access_key
}

output "minio_secret_key" {
  value     = minio_iam_service_account.test_service_account.secret_key
  sensitive = true
}
