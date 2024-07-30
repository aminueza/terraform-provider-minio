resource "minio_iam_group" "developer" {
  name = "developer"
}

resource "minio_iam_user" "user_one" {
  name = "test-user"
}

resource "minio_iam_user" "user_two" {
  name = "test-user-two"
}

resource "minio_iam_group_membership" "developer" {
    name = "tf-testing-group-membership"

  users = [
    "${minio_iam_user.user_one.name}",
    "${minio_iam_user.user_two.name}",
  ]

  group = minio_iam_group.developer.name
}

output "minio_name" {
  value = "${minio_iam_group_membership.developer.id}"
}

output "minio_users" {
  value = "${minio_iam_group_membership.developer.users}"
}

output "minio_group" {
  value = "${minio_iam_group_membership.developer.group}"
}