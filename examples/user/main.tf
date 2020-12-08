terraform {
  required_providers {
    minio = {
      source = "aminueza/minio"
      version = ">= 1.0.0"
    }
  }
  required_version = ">= 0.13"
}

provider "minio" {
  minio_server = var.minio_server
  minio_region = var.minio_region
  minio_access_key = var.minio_access_key
  minio_secret_key = var.minio_secret_key
}

