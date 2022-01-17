resource "minio_iam_user" "test_user" {
  name = "test-user"
}

resource "minio_iam_policy" "test_policy" {
  name = "state-terraform-s3"
  policy= <<EOF
{
  "Version":"2012-10-17",
  "Statement": [
    {
      "Sid":"ListAllBucket",
      "Effect": "Allow",
      "Action": ["s3:PutObject"],
      "Principal":"*",
      "Resource": "arn:aws:s3:::state-terraform-s3/*"
    }
  ]
}
EOF
}

resource "minio_iam_user_policy_attachment" "developer" {
  user_name      = "${minio_iam_user.test_user.id}"
  policy_name = "${minio_iam_policy.test_policy.id}"
}

output "minio_name" {
  value = "${minio_iam_user_policy_attachment.developer.id}"
}

output "minio_users" {
  value = "${minio_iam_user_policy_attachment.developer.user_name}"
}

output "minio_group" {
  value = "${minio_iam_user_policy_attachment.developer.policy_name}"
}