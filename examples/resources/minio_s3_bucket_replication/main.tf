terraform {
  required_providers {
    minio = {
      source  = "aminueza/minio"
      version = ">= 1.19.0"
    }
  }
}

provider "minio" {
  minio_server   = var.minio_server_a
  minio_region   = var.minio_region_a
  minio_user     = var.minio_user_a
  minio_password = var.minio_password_a
}

provider "minio" {
  alias = "deployment_b"
  minio_server   = var.minio_server_b
  minio_region   = var.minio_region_b
  minio_user     = var.minio_user_b
  minio_password = var.minio_password_b
}

