output "minio_user_group" {
  value = "${minio_iam_group_membership.developer.users}"
}
