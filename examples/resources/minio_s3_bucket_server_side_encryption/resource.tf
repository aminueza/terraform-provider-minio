# SSE-S3 encryption (AES256)
resource "minio_s3_bucket_server_side_encryption" "sse_s3" {
  bucket = "my-bucket"

  encryption_type = "AES256"
}

# SSE-KMS encryption
resource "minio_s3_bucket_server_side_encryption" "sse_kms" {
  bucket = "my-kms-bucket"

  encryption_type = "aws:kms"
  kms_key_id      = "my-kms-key"
}
