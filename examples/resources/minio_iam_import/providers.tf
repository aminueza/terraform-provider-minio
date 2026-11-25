provider "minio" {
  alias          = "source"
  minio_server   = "source.example.com:9000"
  minio_user     = "minio"
  minio_password = "minio123"
}

provider "minio" {
  alias          = "target"
  minio_server   = "target.example.com:9000"
  minio_user     = "minio"
  minio_password = "minio456"
}
