ephemeral "vault_kv_secret_v2" "user_password" {
  mount = "secret"
  name  = "minio/iam-user/test-write-only"
}

resource "minio_iam_user" "test" {
  name          = "test"
  force_destroy = true
  tags = {
    tag-key = "tag-value"
  }
}

resource "minio_iam_user" "test_write_only" {
  name              = "test-write-only"
  force_destroy     = true
  secret_wo         = tostring(ephemeral.vault_kv_secret_v2.user_password.data.secret_key)
  secret_wo_version = 1
}

output "test" {
  value = "${minio_iam_user.test.id}"
}

output "status" {
  value = "${minio_iam_user.test.status}"
}

output "secret" {
  value = "${minio_iam_user.test.secret}"
}
