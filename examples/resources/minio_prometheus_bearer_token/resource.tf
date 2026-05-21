resource "minio_prometheus_bearer_token" "cluster" {
  metric_type = "cluster"
  expires_in  = "8760h"
}

output "cluster_token" {
  value     = minio_prometheus_bearer_token.cluster.token
  sensitive = true
}

output "token_expiry" {
  value = minio_prometheus_bearer_token.cluster.token_expiry
}
