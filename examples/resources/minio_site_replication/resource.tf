terraform {
  required_providers {
    minio = {
      source  = "aminueza/minio"
      version = "~> 2"
    }
  }
}

provider "minio" {
  minio_server   = "https://minio1.example.com"
  minio_user     = "minioadmin"
  minio_password = "minio123"
}

# Example: Basic site replication with static configuration
resource "minio_site_replication" "primary" {
  name = "my-cluster-replication"

  site {
    name       = "site1"
    endpoint   = "https://minio1.example.com"
    access_key = "minioadmin"
    secret_key = "minio123"
  }

  site {
    name       = "site2"
    endpoint   = "https://minio2.example.com"
    access_key = "minioadmin"
    secret_key = "minio456"
  }

  site {
    name       = "site3"
    endpoint   = "https://minio3.example.com"
    access_key = "minioadmin"
    secret_key = "minio789"
  }
}
