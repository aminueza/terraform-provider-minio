terraform {
  required_providers {
    minio = {
      source = "aminueza/minio"
    }
  }
}

provider "minio" {
  minio_server   = "localhost:9000"
  minio_user     = "minioadmin"
  minio_password = "minioadmin"
  minio_ssl      = false
}

data "minio_health_status" "cluster" {}

output "cluster_health" {
  value = {
    healthy              = data.minio_health_status.cluster.healthy
    live                 = data.minio_health_status.cluster.live
    ready                = data.minio_health_status.cluster.ready
    write_quorum         = data.minio_health_status.cluster.write_quorum
    read_quorum          = data.minio_health_status.cluster.read_quorum
    safe_for_maintenance = data.minio_health_status.cluster.safe_for_maintenance
  }
}

# Example: Conditional resource creation based on health
resource "minio_s3_bucket" "example" {
  count  = data.minio_health_status.cluster.healthy ? 1 : 0
  bucket = "my-bucket"
}
