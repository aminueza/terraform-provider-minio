output "minio_user" {
  value = minio_iam_service_account.test_service_account.access_key
}

output "minio_password" {
  value     = minio_iam_service_account.test_service_account.secret_key
  sensitive = true
}
