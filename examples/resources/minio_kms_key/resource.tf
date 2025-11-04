# Create a KMS encryption key
resource "minio_kms_key" "example" {
  key_id = "my-encryption-key"
}
