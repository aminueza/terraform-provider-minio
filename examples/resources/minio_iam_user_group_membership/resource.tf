resource "minio_iam_user" "alice" {
  name = "alice"
}

resource "minio_iam_group" "dev" {
  name = "developers"
}

resource "minio_iam_group" "ops" {
  name = "operations"
}

resource "minio_iam_user_group_membership" "alice" {
  user = minio_iam_user.alice.name

  groups = [
    minio_iam_group.dev.name,
    minio_iam_group.ops.name,
  ]
}
