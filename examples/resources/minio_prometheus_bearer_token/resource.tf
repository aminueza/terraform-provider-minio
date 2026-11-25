# Create a bearer token for cluster metrics
resource "minio_prometheus_bearer_token" "cluster" {
  metric_type = "cluster"
  expires_in  = "24h"
}

# Create a bearer token for node metrics with custom expiry
resource "minio_prometheus_bearer_token" "node" {
  metric_type = "node"
  expires_in  = "168h" # 1 week
}

# Create a bearer token for bucket metrics with limit
resource "minio_prometheus_bearer_token" "bucket" {
  metric_type = "bucket"
  expires_in  = "720h" # 30 days
  limit       = 2160   # 90 days limit
}

# Create a bearer token for resource metrics
resource "minio_prometheus_bearer_token" "resource" {
  metric_type = "resource"
  expires_in  = "87600h" # 10 years (default)
}
