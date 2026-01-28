data "minio_iam_user" "example" {
  name = "existing-user"
}

output "user_status" {
  value = data.minio_iam_user.example.status
}

output "user_details" {
  value = {
    name   = data.minio_iam_user.example.id
    status = data.minio_iam_user.example.status
  }
}

resource "minio_iam_user_policy_attachment" "conditional" {
  count = data.minio_iam_user.example.status == "enabled" ? 1 : 0

  user_name   = data.minio_iam_user.example.id
  policy_name = "readonly-policy"
}
