data "minio_s3_bucket_anonymous_access" "example" {
  bucket = "public-assets"
}

output "policy" {
  value = data.minio_s3_bucket_anonymous_access.example.policy
}

output "access_type" {
  value = data.minio_s3_bucket_anonymous_access.example.access_type
}
