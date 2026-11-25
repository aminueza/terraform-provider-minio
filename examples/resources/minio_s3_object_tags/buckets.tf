# Buckets whose objects are tagged in resource.tf.

resource "minio_s3_bucket" "data" {
  bucket = "data"
}

resource "minio_s3_bucket" "logs" {
  bucket = "logs"
}
