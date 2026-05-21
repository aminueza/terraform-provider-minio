resource "minio_prometheus_bearer_token" "cluster" {
  metric_type = "cluster"
}

data "minio_prometheus_scrape_config" "cluster" {
  metric_type  = "cluster"
  bearer_token = minio_prometheus_bearer_token.cluster.token
}

output "scrape_config_yaml" {
  value     = data.minio_prometheus_scrape_config.cluster.scrape_config
  sensitive = true
}
