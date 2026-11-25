# Buckets the CORS configurations in resource.tf attach to.

resource "minio_s3_bucket" "uploads" {
  bucket = "uploads"
}

resource "minio_s3_bucket" "shared" {
  bucket = "shared"
}

resource "minio_s3_bucket" "user_uploads" {
  bucket = "user-uploads"
}

resource "minio_s3_bucket" "dev_bucket" {
  bucket = "dev-bucket"
}
