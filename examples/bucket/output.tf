output "minio_id" {
  value = minio_s3_bucket.state_terraform_s3.id
}

output "minio_url" {
  value = minio_s3_bucket.state_terraform_s3.bucket_domain_name
}