ephemeral "vault_kv_secret_v2" "service_account_password" {
  mount = "secret"
  name  = "minio/iam-service-account/test-write-only"
}

resource "minio_iam_user" "test" {
  name          = "test"
  force_destroy = true
  tags = {
    tag-key = "tag-value"
  }
}

resource "minio_iam_service_account" "test_service_account" {
  target_user = minio_iam_user.test.name
}

resource "minio_iam_service_account" "test_service_account_write_only" {
  target_user           = minio_iam_user.test.name
  secret_key_wo         = tostring(ephemeral.vault_kv_secret_v2.service_account_password.data.secret_key)
  secret_key_wo_version = 1
}

output "minio_user" {
  value = minio_iam_service_account.test_service_account.access_key
}

output "minio_password" {
  value     = minio_iam_service_account.test_service_account.secret_key
  sensitive = true
}
