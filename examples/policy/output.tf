
output "minio_id" {
  value = minio_iam_policy.test_policy.id
}

output "minio_policy" {
  value = minio_iam_policy.test_policy.policy
}