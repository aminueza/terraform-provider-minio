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

output "minio_id" {
  value = "${minio_iam_policy.test_policy.id}"
}

output "minio_policy" {
  value = minio_iam_policy.test_policy.policy
}