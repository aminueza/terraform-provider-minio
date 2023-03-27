resource "minio_iam_group" "developer" {
  name = "developer"
}

resource "minio_iam_group_policy" "test_policy" {
  name   = "state-terraform-s3"
  policy = <<EOF
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

resource "minio_iam_group_policy_attachment" "developer" {
  group_name  = minio_iam_group.group.name
  policy_name = minio_iam_policy.test_policy.id
}

output "minio_name" {
  value = minio_iam_group_policy_attachment.developer.id
}

output "minio_users" {
  value = minio_iam_group_policy_attachment.developer.group_name
}

output "minio_group" {
  value = minio_iam_group_policy_attachment.developer.policy_name
}


# Example using an LDAP Group instead of a static MinIO group

resource "minio_iam_group_policy_attachment" "developer" {
  user_name   = "OU=Unit,DC=example,DC=com"
  policy_name = "${minio_iam_policy.test_policy.id}"
}