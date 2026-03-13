ephemeral "vault_kv_secret_v2" "access_key_password" {
  mount = "secret"
  name  = "minio/accesskey/custom-key-write-only"
}

# Create a user first
resource "minio_iam_user" "example_user" {
  name = "example-user"
}

# Create an access key with default settings (auto-generated keys, enabled)
resource "minio_accesskey" "example" {
  user = minio_iam_user.example_user.name
}

# Create an access key with custom credentials
resource "minio_accesskey" "custom_key" {
  user               = minio_iam_user.example_user.name
  access_key         = "MINIO_ACCESS_KEY" # Must be 8-20 characters
  secret_key         = "mySuperSecretKey" # Must be at least 8 characters
  secret_key_version = "v1"               # Version identifier for change detection
  status             = "enabled"
}

# Create an access key using write-only secret
resource "minio_accesskey" "custom_key_write_only" {
  user                  = minio_iam_user.example_user.name
  access_key            = "MINIO_ACCESS_KEY2"
  secret_key_wo         = tostring(ephemeral.vault_kv_secret_v2.access_key_password.data.secret_key)
  secret_key_wo_version = 1
  status                = "enabled"
}
