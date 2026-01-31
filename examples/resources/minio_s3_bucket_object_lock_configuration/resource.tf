# Create a bucket with object locking enabled
resource "minio_s3_bucket" "compliance_bucket" {
  bucket         = "compliance-data"
  object_locking = true
}

# Configure default retention policy with days
resource "minio_s3_bucket_object_lock_configuration" "compliance" {
  bucket              = minio_s3_bucket.compliance_bucket.bucket
  object_lock_enabled = "Enabled"

  rule {
    default_retention {
      mode = "COMPLIANCE"
      days = 2555 # 7 years in days
    }
  }
}

# Example with years for financial records
resource "minio_s3_bucket" "financial_records" {
  bucket         = "financial-records"
  object_locking = true
}

resource "minio_s3_bucket_object_lock_configuration" "financial" {
  bucket = minio_s3_bucket.financial_records.bucket

  rule {
    default_retention {
      mode  = "COMPLIANCE"
      years = 7
    }
  }
}

# Example with GOVERNANCE mode for backups
resource "minio_s3_bucket" "backup_bucket" {
  bucket         = "daily-backups"
  object_locking = true
}

resource "minio_s3_bucket_object_lock_configuration" "backups" {
  bucket = minio_s3_bucket.backup_bucket.bucket

  rule {
    default_retention {
      mode = "GOVERNANCE"
      days = 90
    }
  }
}
