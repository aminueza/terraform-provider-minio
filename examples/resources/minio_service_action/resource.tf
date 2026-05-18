resource "minio_service_action" "restart" {
  action = "restart"
  triggers = {
    when = "manual-trigger"
  }
}
