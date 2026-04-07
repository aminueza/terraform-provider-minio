resource "minio_s3_bucket" "state_terraform_s3" {
  bucket = "state-terraform-s3"
  acl    = "public"
}

resource "minio_s3_object" "txt_file" {
  depends_on   = [minio_s3_bucket.state_terraform_s3]
  bucket_name  = minio_s3_bucket.state_terraform_s3.bucket
  object_name  = "text.txt"
  content      = "Lorem ipsum dolor sit amet."
  content_type = "text/plain"
}

resource "minio_s3_object" "public_file" {
  depends_on   = [minio_s3_bucket.state_terraform_s3]
  bucket_name  = minio_s3_bucket.state_terraform_s3.bucket
  object_name  = "public.txt"
  content      = "This file is publicly readable."
  content_type = "text/plain"
  acl          = "public-read"
}

resource "minio_s3_object" "with_metadata" {
  depends_on          = [minio_s3_bucket.state_terraform_s3]
  bucket_name         = minio_s3_bucket.state_terraform_s3.bucket
  object_name         = "report.json"
  content             = "{}"
  content_type        = "application/json"
  content_encoding    = "gzip"
  content_disposition = "attachment; filename=\"report.json\""
  cache_control       = "max-age=3600"
  storage_class       = "STANDARD"

  metadata = {
    environment = "production"
    team        = "platform"
  }
}

output "minio_id" {
  value = minio_s3_object.txt_file.id
}