# Generate scrape configuration for cluster metrics
data "minio_prometheus_scrape_config" "cluster" {
  metric_type = "cluster"
  alias       = "minio-cluster"
}

# Generate scrape configuration for node metrics
data "minio_prometheus_scrape_config" "node" {
  metric_type = "node"
  alias       = "minio-nodes"
}

# Generate scrape configuration for bucket metrics with v2 metrics
data "minio_prometheus_scrape_config" "bucket" {
  metric_type     = "bucket"
  alias           = "minio-buckets"
  metrics_version = "v2"
}

# Generate scrape configuration for resource metrics with bearer token
data "minio_prometheus_scrape_config" "resource" {
  metric_type  = "resource"
  alias        = "minio-resources"
  bearer_token = "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJub3RlIjoiZXhhbXBsZS10b2tlbiJ9.example_signature"
}
