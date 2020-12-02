output "user_minio_user" {
  value = minio_iam_user.minio_user.id
}

output "minio_user_status" {
  value = minio_iam_user.minio_user.status
}

output "minio_user_secret" {
  value = minio_iam_user.minio_user.secret
}