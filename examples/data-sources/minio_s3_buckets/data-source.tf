data "minio_s3_buckets" "all" {}

output "bucket_names" {
  value = data.minio_s3_buckets.all.buckets[*].name
}
