terraform {
  required_providers {
    minio = {
      source  = "aminueza/minio"
      version = "~> 3"
    }
    vault = {
      source  = "hashicorp/vault"
      version = "~> 5"
    }
  }
}

provider "minio" {
  minio_server   = "https://minio1.example.com"
  minio_user     = "minioadmin"
  minio_password = "minio123"
}
