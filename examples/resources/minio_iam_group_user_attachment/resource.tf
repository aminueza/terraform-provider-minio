resource "minio_iam_group" "developer" {
  name = "developer"
}
resource "minio_iam_user" "user_one" {
  name = "test-user"
}

resource "minio_iam_group_user_attachment" "developer" {
  group_name = minio_iam_group.developer.name
  user_name  = minio_iam_user.user_one.name
}

output "minio_name" {
  value = minio_iam_group_user_attachment.developer.id
}

output "minio_group" {
  value = minio_iam_group_user_attachment.developer.group_name
}

output "minio_user" {
  value = minio_iam_group_user_attachment.developer.user_name
}