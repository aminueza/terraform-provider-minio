# S3-compatible remote tier
resource "minio_ilm_tier" "s3_archive" {
  name     = "S3ARCHIVE"
  type     = "s3"
  bucket   = "archive-bucket"
  prefix   = "minio-data/"
  endpoint = "s3.amazonaws.com"
  region   = "us-east-1"

  s3_config {
    access_key    = var.aws_access_key
    secret_key    = var.aws_secret_key
    storage_class = "GLACIER"
  }
}

# Remote MinIO tier
resource "minio_ilm_tier" "remote" {
  name     = "REMOTEMINIO"
  type     = "minio"
  bucket   = "tier-storage"
  endpoint = "minio2.example.com"

  minio_config {
    access_key = var.remote_access_key
    secret_key = var.remote_secret_key
  }
}

# Azure Blob Storage tier
resource "minio_ilm_tier" "azure" {
  name   = "AZURECOOL"
  type   = "azure"
  bucket = "archive-container"

  azure_config {
    account_name = var.azure_account
    account_key  = var.azure_key
  }
}

# GCS tier
resource "minio_ilm_tier" "gcs" {
  name   = "GCSNEARLINE"
  type   = "gcs"
  bucket = "gcs-archive"

  gcs_config {
    credentials   = file("gcs-service-account.json")
    storage_class = "NEARLINE"
  }
}
