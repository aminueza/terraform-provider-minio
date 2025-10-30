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

# Configure server region
resource "minio_config" "region" {
  key   = "region"
  value = "name=us-west-1"
}

# Configure API request limits
resource "minio_config" "api_settings" {
  key   = "api"
  value = "requests_max=1000 requests_deadline=10s"
}

# Configure webhook notification endpoint
resource "minio_config" "webhook_notification" {
  key   = "notify_webhook:production"
  value = "endpoint=http://webhook.example.com/events queue_limit=1000"
}

# Output restart requirement status
output "region_restart_required" {
  value       = minio_config.region.restart_required
  description = "Whether MinIO server restart is required for region config"
}

output "api_restart_required" {
  value       = minio_config.api_settings.restart_required
  description = "Whether MinIO server restart is required for API config"
}
