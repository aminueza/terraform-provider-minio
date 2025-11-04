# Configure server-side encryption for a bucket
resource "minio_s3_bucket_server_side_encryption" "example" {
  bucket = "my-bucket"

  encryption_type = "aws:kms"
  kms_key_id      = "my-kms-key"
}
