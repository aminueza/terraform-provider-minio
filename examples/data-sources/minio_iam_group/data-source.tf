data "minio_iam_group" "admins" {
  name = "admins"
}

output "admin_members" {
  value = data.minio_iam_group.admins.members
}
