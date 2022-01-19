resource "minio_s3_bucket" "bucket" {
  bucket = "bucket"
}

resource "minio_ilm_rule" "bucket-lifecycle-rules" {
  bucket = "bucket"

  rules {
    id         = "expire-7d"
    expiration = 7
  }
}
