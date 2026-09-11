ephemeral "minio_sts_credentials" "scoped" {
  session_name     = "terraform-scoped"
  duration_seconds = 3600

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject", "s3:PutObject"]
      Resource = ["arn:aws:s3:::uploads/*"]
    }]
  })
}

provider "minio" {
  alias = "scoped"

  minio_server        = var.minio_server
  minio_user          = ephemeral.minio_sts_credentials.scoped.access_key
  minio_password      = ephemeral.minio_sts_credentials.scoped.secret_key
  minio_session_token = ephemeral.minio_sts_credentials.scoped.session_token
}
