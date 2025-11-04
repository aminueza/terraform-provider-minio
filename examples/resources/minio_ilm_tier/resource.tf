# Configure S3-compatible remote tier for ILM
resource "minio_ilm_tier" "s3_tier" {
  name = "S3TIER"
  type = "s3"

  s3 {
    endpoint    = "s3.amazonaws.com"
    bucket      = "remote-tier-bucket"
    prefix      = "minio-data/"
    region      = "us-east-1"
    access_key  = "AWS_ACCESS_KEY"
    secret_key  = "AWS_SECRET_KEY"
    storage_class = "STANDARD_IA"
  }
}
