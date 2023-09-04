resource "minio_iam_user" "test_user" {
  name = "test-user"
}

resource "minio_iam_service_account" "test_service_account" {
  target_user = "test-user"
  policy = <<-EOF
    {
    "Version":"2012-10-17",
    "Statement": [
        {
        "Sid":"ListAllBucket",
        "Effect": "Allow",
        "Action": "*",
        "Principal":"*",
        "Resource": "arn:aws:s3:::*"
        }
    ]
    }
  EOF
}

output "minio_service_account_key" {
  value = minio_iam_service_account.test_service_account.access_key
}

output "minio_service_account_secret" {
  value     = minio_iam_service_account.test_service_account.secret_key
  sensitive = true
}
