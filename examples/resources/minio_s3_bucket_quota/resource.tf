resource "minio_s3_bucket_quota" "example" {
  bucket = "example-bucket"
  quota  = 1073741824 # 1 GiB
  type   = "hard"
}
