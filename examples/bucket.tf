resource "minio_bucket" "state_terraform_s3" {
  bucket = "state-terraform-s3"
  acl    = "public-write"
  debug = false
}

