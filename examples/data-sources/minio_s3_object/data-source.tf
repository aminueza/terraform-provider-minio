data "minio_s3_object" "document" {
  object_name = "document.txt"
  bucket_name = "documents-bucket"
}
