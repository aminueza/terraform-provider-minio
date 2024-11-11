resource "minio_s3_bucket" "test_bucket" {
  bucket          = "test-retention-bucket"
  force_destroy   = true
  object_locking  = false
}

# Add basic retention configuration
resource "minio_s3_bucket_retention" "test_retention" {
  bucket          = minio_s3_bucket.test_bucket.bucket
  mode            = "GOVERNANCE"
  unit            = "YEARS"
  validity_period = 1
}
