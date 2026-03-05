data "minio_s3_bucket" "my_bucket" {
  bucket = "my-bucket"
}

output "versioning_enabled" {
  value = data.minio_s3_bucket.my_bucket.versioning_enabled
}
