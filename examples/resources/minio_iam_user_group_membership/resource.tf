resource "minio_iam_user" "user" {
  name = "example-user"
}

resource "minio_iam_group" "developers" {
  name = "developers"
}

resource "minio_iam_group" "admins" {
  name = "admins"
}

resource "minio_iam_user_group_membership" "example" {
  user = minio_iam_user.user.name

  groups = [
    minio_iam_group.developers.name,
    minio_iam_group.admins.name,
  ]
}
