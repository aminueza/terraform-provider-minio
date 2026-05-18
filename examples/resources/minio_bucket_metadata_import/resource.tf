data "minio_bucket_metadata_export" "source" {
  bucket = "source-bucket"
}

resource "minio_bucket_metadata_import" "example" {
  bucket   = "target-bucket"
  metadata = data.minio_bucket_metadata_export.source.metadata
}
