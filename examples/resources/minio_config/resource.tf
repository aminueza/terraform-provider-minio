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
