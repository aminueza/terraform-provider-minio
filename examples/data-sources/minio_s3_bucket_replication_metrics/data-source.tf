data "minio_s3_bucket_replication_metrics" "source" {
  bucket = "my-replicated-bucket"
}

output "replication_pending_bytes" {
  value = data.minio_s3_bucket_replication_metrics.source.pending_size
}

output "replication_failed_count" {
  value = data.minio_s3_bucket_replication_metrics.source.failed_count
}
