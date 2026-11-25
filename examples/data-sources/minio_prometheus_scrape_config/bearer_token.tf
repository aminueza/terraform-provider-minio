# Create a bearer token for authenticated access
resource "minio_prometheus_bearer_token" "cluster_token" {
  metric_type = "cluster"
  expires_in  = "24h"
}

# Generate authenticated scrape configuration
data "minio_prometheus_scrape_config" "cluster_auth" {
  metric_type  = "cluster"
  alias        = "minio-cluster-auth"
  bearer_token = minio_prometheus_bearer_token.cluster_token.token
}

# Output the complete Prometheus configuration
output "prometheus_config" {
  value     = data.minio_prometheus_scrape_config.cluster_auth.scrape_config
  sensitive = true
}
