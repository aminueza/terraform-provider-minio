resource "minio_s3_bucket" "data" {
  bucket         = "my-data-bucket"
  object_locking = true
}

resource "minio_s3_object" "document" {
  bucket_name = minio_s3_bucket.data.id
  object_name = "important/document.pdf"
  content     = "important content"
}

resource "minio_s3_object_legal_hold" "hold" {
  bucket = minio_s3_bucket.data.id
  key    = minio_s3_object.document.object_name
  status = "ON"
}
