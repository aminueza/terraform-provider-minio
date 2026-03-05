data "minio_iam_policy" "readonly" {
  name = "readonly"
}

output "policy_json" {
  value = data.minio_iam_policy.readonly.policy
}
