# Basic CORS configuration for web application
resource "minio_s3_bucket_cors" "webapp" {
  bucket = minio_s3_bucket.uploads.id

  cors_rule {
    allowed_origins = ["https://app.example.com", "https://www.example.com"]
    allowed_methods = ["GET", "PUT", "POST", "DELETE"]
    allowed_headers = ["*"]
    expose_headers  = ["ETag", "x-amz-meta-custom"]
    max_age_seconds = 3600
  }
}

# Multiple CORS rules for different applications
resource "minio_s3_bucket_cors" "multi_app" {
  bucket = minio_s3_bucket.shared.id

  cors_rule {
    id              = "allow-webapp"
    allowed_origins = ["https://app.example.com"]
    allowed_methods = ["GET", "PUT", "POST"]
    allowed_headers = ["Content-Type", "Authorization"]
    expose_headers  = ["ETag"]
    max_age_seconds = 3600
  }

  cors_rule {
    id              = "allow-mobile"
    allowed_origins = ["https://mobile.example.com"]
    allowed_methods = ["GET", "PUT"]
    allowed_headers = ["Content-Type"]
    max_age_seconds = 7200
  }
}

# CORS configuration for direct browser uploads
resource "minio_s3_bucket_cors" "direct_upload" {
  bucket = minio_s3_bucket.user_uploads.id

  cors_rule {
    allowed_origins = ["https://app.example.com"]
    allowed_methods = ["PUT", "POST"]
    allowed_headers = [
      "Content-Type",
      "Content-MD5",
      "x-amz-meta-*",
    ]
    expose_headers  = ["ETag"]
    max_age_seconds = 3600
  }
}

# Permissive CORS configuration for development
resource "minio_s3_bucket_cors" "dev" {
  bucket = minio_s3_bucket.dev_bucket.id

  cors_rule {
    allowed_origins = ["*"]
    allowed_methods = ["GET", "PUT", "POST", "DELETE", "HEAD"]
    allowed_headers = ["*"]
    max_age_seconds = 3600
  }
}
