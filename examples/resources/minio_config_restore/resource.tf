data "minio_config_history" "recent" {
  limit = 10
}

resource "minio_config_restore" "rollback" {
  restore_id = data.minio_config_history.recent.entries[0].restore_id
}