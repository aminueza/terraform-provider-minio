# Enable versioning on a bucket
resource "minio_s3_bucket_versioning" "example" {
  bucket = "my-bucket"

  versioning_configuration {
    status = "Enabled"
  }
}
