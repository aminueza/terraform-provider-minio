data "minio_iam_export" "source_cluster" {
  provider = minio.source
}

resource "minio_iam_import" "target_cluster" {
  provider = minio.target

  iam_data = data.minio_iam_export.source_cluster.iam_data
}

output "imported_users" {
  value = minio_iam_import.target_cluster.added_users
}

output "imported_policies" {
  value = minio_iam_import.target_cluster.added_policies
}
