data "minio_bucket_metadata_export" "example" {
  bucket = "my-bucket"
}

output "metadata" {
  value = data.minio_bucket_metadata_export.example.metadata
}
