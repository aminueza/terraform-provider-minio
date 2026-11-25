resource "minio_s3_bucket" "data_lake" {
  bucket = "my-data-lake"

  tags = {
    Environment = "production"
    Team        = "data-engineering"
    CostCenter  = "12345"
  }
}
