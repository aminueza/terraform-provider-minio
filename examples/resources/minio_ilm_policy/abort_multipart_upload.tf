# Example of using abort_incomplete_multipart_upload in minio_ilm_policy

resource "minio_s3_bucket" "example" {
  bucket = "example-bucket"
  acl    = "private"
}

resource "minio_ilm_policy" "abort_mpu_example" {
  bucket = minio_s3_bucket.example.bucket

  rule {
    id = "abort-incomplete-mpu"
    abort_incomplete_multipart_upload {
      days_after_initiation = "7d"
    }
  }
}

# Example combining abort_incomplete_multipart_upload with other lifecycle rules
resource "minio_ilm_policy" "comprehensive_example" {
  bucket = minio_s3_bucket.example.bucket

  rule {
    id = "cleanup-rule"
    status = "Enabled"
    
    # Abort incomplete multipart uploads after 3 days
    abort_incomplete_multipart_upload {
      days_after_initiation = "3d"
    }
    
    # Expire regular objects after 30 days
    expiration = "30d"
    
    # Apply only to uploads/ prefix
    filter = "uploads/"
  }
}
