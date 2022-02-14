resource "minio_s3_bucket" "bucket" {
  bucket = "bucket"
}

resource "minio_ilm_policy" "bucket-lifecycle-rules" {
  bucket = minio_s3_bucket.bucket.bucket

  rule {
    id         = "expire-7d"
    expiration = 7
  }
}
