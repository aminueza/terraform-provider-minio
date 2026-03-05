data "minio_iam_groups" "all" {}

output "group_names" {
  value = data.minio_iam_groups.all.groups[*].name
}
