resource "minio_service_action" "freeze" {
  action = "freeze"
  triggers = {
    when = "manual-trigger"
  }
}
