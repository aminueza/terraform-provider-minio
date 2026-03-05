data "minio_s3_bucket_tags" "my_bucket" {
  bucket = "my-bucket"
}

output "environment" {
  value = data.minio_s3_bucket_tags.my_bucket.tags["Environment"]
}
