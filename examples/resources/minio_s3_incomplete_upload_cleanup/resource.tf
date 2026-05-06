resource "minio_s3_bucket" "example" {
  bucket = "my-bucket"
}

resource "minio_s3_incomplete_upload_cleanup" "cleanup" {
  bucket = minio_s3_bucket.example.bucket
  prefix = "uploads/"  # Optional: only clean up uploads with this prefix
}
