resource "minio_iam_group" "developer" {
  name = "developer"
}

output "minio_user_group" {
  value = minio_iam_group.developer.group_name
}