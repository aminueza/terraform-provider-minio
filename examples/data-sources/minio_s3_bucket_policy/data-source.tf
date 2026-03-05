data "minio_s3_bucket_policy" "my_bucket" {
  bucket = "my-bucket"
}

output "policy_json" {
  value = data.minio_s3_bucket_policy.my_bucket.policy
}
