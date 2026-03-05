data "minio_iam_user_policies" "audit" {
  name = "app-user"
}

output "all_effective_policies" {
  value = data.minio_iam_user_policies.audit.all_policies
}

output "inherited_from_groups" {
  value = data.minio_iam_user_policies.audit.group_policies
}
