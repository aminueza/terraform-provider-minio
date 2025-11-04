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
