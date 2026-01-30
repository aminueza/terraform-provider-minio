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

# Bucket name prefix (create-only)
resource "minio_s3_bucket" "customer" {
  bucket_prefix = "customer-"
  acl           = "private"
}

# Bucket name prefix for globally-unique buckets while keeping existing buckets via migration
resource "minio_s3_bucket" "globally_unique_bucket" {
  bucket_prefix = "globally-unique-bucket-"
  acl           = "private"
}

output "minio_id" {
  value = minio_s3_bucket.state_terraform_s3.id
}

output "minio_url" {
  value = minio_s3_bucket.state_terraform_s3.bucket_domain_name
}