resource "minio_s3_bucket" "example" {
  bucket = "public-assets"
}

# Grant read-only anonymous access using canned access_type
resource "minio_s3_bucket_anonymous_access" "read_only" {
  bucket      = minio_s3_bucket.example.id
  access_type = "public-read"
}

# Provide a fully custom anonymous policy
resource "minio_s3_bucket_anonymous_access" "custom" {
  bucket = minio_s3_bucket.example.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect    = "Allow"
        Principal = { AWS = ["*"] }
        Action    = ["s3:GetObject"]
        Resource  = ["${minio_s3_bucket.example.arn}/*"]
      }
    ]
  })
}
