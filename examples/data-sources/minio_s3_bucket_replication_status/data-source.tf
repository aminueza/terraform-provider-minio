data "minio_s3_bucket_replication_status" "source" {
  bucket = "my-replicated-bucket"
}

output "replication_rules" {
  value = data.minio_s3_bucket_replication_status.source.rules
}
