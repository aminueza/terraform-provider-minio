resource "minio_s3_bucket" "example" {
  bucket = "my-app-data"
  acl    = "private"
}

resource "minio_s3_bucket_versioning" "example" {
  bucket = minio_s3_bucket.example.bucket
  versioning_configuration {
    status = "Enabled"
  }
}

resource "minio_s3_bucket_lifecycle" "example" {
  bucket = minio_s3_bucket_versioning.example.bucket

  rule {
    id = "expire-logs-after-30-days"
    filter {
      prefix = "logs/"
    }
    expiration {
      days = 30
    }
    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }

  rule {
    id = "tiered-archival-for-reports"
    filter {
      and {
        prefix = "reports/"
        tags = {
          retain = "long-term"
        }
      }
    }
    transition {
      days          = 60
      storage_class = "GLACIER"
    }
    noncurrent_version_expiration {
      noncurrent_days           = 180
      newer_noncurrent_versions = 3
    }
  }

  rule {
    id     = "cleanup-temporary-large-objects"
    status = "Enabled"
    filter {
      object_size_greater_than = 10485760 # 10 MiB
    }
    expiration {
      days = 7
    }
  }
}
