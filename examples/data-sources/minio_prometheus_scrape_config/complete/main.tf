# Generate scrape configs for all metric types
data "minio_prometheus_scrape_config" "cluster" {
  metric_type = "cluster"
  alias       = "minio-cluster"
}

data "minio_prometheus_scrape_config" "node" {
  metric_type = "node"
  alias       = "minio-nodes"
}

data "minio_prometheus_scrape_config" "bucket" {
  metric_type = "bucket"
  alias       = "minio-buckets"
}

data "minio_prometheus_scrape_config" "resource" {
  metric_type = "resource"
  alias       = "minio-resources"
}

# Combine all scrape configs
locals {
  prometheus_config = <<-EOT
    ${data.minio_prometheus_scrape_config.cluster.scrape_config}

    ${data.minio_prometheus_scrape_config.node.scrape_config}

    ${data.minio_prometheus_scrape_config.bucket.scrape_config}

    ${data.minio_prometheus_scrape_config.resource.scrape_config}
  EOT
}

output "complete_prometheus_config" {
  value     = local.prometheus_config
  sensitive = true
}
