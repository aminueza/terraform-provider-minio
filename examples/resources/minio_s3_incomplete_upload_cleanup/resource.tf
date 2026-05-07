resource "minio_s3_bucket" "example" {
  bucket = "my-bucket"
}

# Clean up once on first apply
resource "minio_s3_incomplete_upload_cleanup" "cleanup" {
  bucket = minio_s3_bucket.example.bucket
  prefix = "uploads/" # Optional: only clean up uploads with this prefix
}

# Re-run cleanup by changing the trigger value (e.g. via -var or a local)
resource "minio_s3_incomplete_upload_cleanup" "scheduled" {
  bucket = minio_s3_bucket.example.bucket
  triggers = {
    run = "2024-01-01" # Change this value to trigger a new cleanup run
  }
}
