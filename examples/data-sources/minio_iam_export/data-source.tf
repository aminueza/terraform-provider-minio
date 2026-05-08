data "minio_iam_export" "backup" {}

resource "local_sensitive_file" "iam_backup" {
  filename       = "${path.module}/iam-backup-${data.minio_iam_export.backup.sha256}.zip.b64"
  content        = data.minio_iam_export.backup.iam_data
  file_permission = "0600"
}

output "iam_backup_size" {
  value = data.minio_iam_export.backup.size_bytes
}
