# Bucket protected from accidental deletion (default behavior)
resource "minio_s3_bucket" "state_terraform_s3" {
  bucket = "state-terraform-s3"
  acl    = "public"
  # force_destroy defaults to false - bucket deletion will fail if not empty
}

# Bucket that can be destroyed even with objects inside
resource "minio_s3_bucket" "temporary_data" {
  bucket        = "temporary-data"
  acl           = "private"
  force_destroy = true
}

output "minio_id" {
  value = minio_s3_bucket.state_terraform_s3.id
}

output "minio_url" {
  value = minio_s3_bucket.state_terraform_s3.bucket_domain_name
}