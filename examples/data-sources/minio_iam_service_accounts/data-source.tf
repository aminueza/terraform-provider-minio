data "minio_iam_service_accounts" "app" {
  user = "app-user"
}

output "service_account_keys" {
  value = data.minio_iam_service_accounts.app.service_accounts[*].access_key
}
