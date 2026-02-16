resource "minio_s3_object_tags" "config_file" {
  bucket = minio_s3_bucket.data.id
  key    = "config/app.json"

  tags = {
    Type        = "configuration"
    Application = "webapp"
    Version     = "1.0"
  }
}

resource "minio_s3_object_tags" "logs" {
  bucket = minio_s3_bucket.logs.id
  key    = "application.log"

  tags = {
    LogType     = "application"
    Environment = "production"
  }
}