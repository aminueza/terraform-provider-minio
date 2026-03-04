resource "minio_s3_bucket_replication_resync" "sync_existing" {
  bucket = minio_s3_bucket.source.id

  depends_on = [minio_s3_bucket_replication.to_target]
}
