resource "minio_s3_bucket" "bucket" {
  bucket = "bucket"
}

resource "minio_ilm_policy" "test_policy" {
  bucket = minio_s3_bucket.bucket.bucket

  rule {
    id     = "rule1"
    status = "Enabled"
    # Delete objects after 90 days
    expiration = "90d"
    filter     = "documents/"
    tags = {
      environment = "test"
      type        = "document"
    }
  }

  rule {
    id     = "rule2"
    status = "Disabled"
    # Move objects to GLACIER after 30 days
    transition {
      days          = "30d"
      storage_class = "GLACIER"
    }
    filter = "backups/"
  }

  rule {
    id = "rule3"
    # Transition on a specific date instead of an age
    transition {
      date          = "2027-12-31"
      storage_class = "STANDARD_IA"
    }
    filter = "archives/"
  }

  rule {
    id = "rule4"
    # Handle noncurrent versions
    noncurrent_transition {
      days           = "45d"
      storage_class  = "GLACIER"
      newer_versions = 3
    }
    filter = "versioned-data/"
  }

  rule {
    id = "rule5"
    # Delete old versions
    noncurrent_expiration {
      days           = "365d"
      newer_versions = 5
    }
    filter = "old-versions/"
  }

  rule {
    id = "rule6"
    # Combined policy: transition then expire
    transition {
      days          = "60d"
      storage_class = "STANDARD_IA"
    }
    expiration = "180d"
    filter     = "logs/"
    tags = {
      retention = "short-term"
      type      = "logs"
    }
  }

  rule {
    id = "rule7"
    # Remove expired delete markers, and drop noncurrent versions after 30 days
    expired_object_delete_marker = true
    noncurrent_expiration {
      days = "30d"
    }
    filter = "versioned/"
  }
}
