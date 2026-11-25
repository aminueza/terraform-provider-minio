# Generate bearer token
resource "minio_prometheus_bearer_token" "cluster_metrics" {
  metric_type = "cluster"
  expires_in  = "24h"
}

# Generate scrape configuration with bearer token
data "minio_prometheus_scrape_config" "cluster" {
  metric_type  = "cluster"
  bearer_token = minio_prometheus_bearer_token.cluster_metrics.token
}

output "prometheus_scrape_config" {
  value     = data.minio_prometheus_scrape_config.cluster.scrape_config
  sensitive = true
}
