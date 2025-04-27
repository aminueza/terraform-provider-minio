terraform {
  required_providers {
    minio = {
      source = "aminueza/minio"
    }
  }
}

provider "minio" {
  minio_server   = "localhost:9000"
  minio_user     = "minioadmin"
  minio_password = "minioadmin"
  minio_ssl      = false
}

# Create a user first
resource "minio_iam_user" "test_user" {
  name = "test-user"
}

# Then create an accesskey for that user
resource "minio_accesskey" "test_key" {
  user   = minio_iam_user.test_user.name
  status = "enabled"
  # Optionally specify access_key and secret_key
  # access_key = "custom_access_key"
  # secret_key = "custom_secret_key"
}

# If you want to attach a policy to the user
resource "minio_iam_policy" "test_policy" {
  name = "test-policy"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "s3:ListBucket",
          "s3:GetObject",
        ]
        Effect   = "Allow"
        Resource = ["arn:aws:s3:::*"]
      }
    ]
  })
}

resource "minio_iam_user_policy_attachment" "test_attachment" {
  user_name   = minio_iam_user.test_user.name
  policy_name = minio_iam_policy.test_policy.id
}

# Output the generated access and secret keys
output "access_key" {
  value = minio_accesskey.test_key.access_key
}

output "secret_key" {
  value     = minio_accesskey.test_key.secret_key
  sensitive = true
}
