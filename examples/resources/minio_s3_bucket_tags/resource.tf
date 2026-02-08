resource "minio_s3_bucket_tags" "example" {
  bucket = "example-bucket"
  tags = {
    Environment = "dev"
    Owner       = "team-a"
  }
}
