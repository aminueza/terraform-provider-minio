resource "minio_iam_policy" "test_policy" {
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

resource "minio_iam_ldap_user_policy_attachment" "developer" {
  user_dn    = "CN=developer,OU=Unit,DC=example,DC=com"
  policy_name = minio_iam_policy.test_policy.id
}

# Example using a builtin policy
resource "minio_iam_ldap_user_policy_attachment" "admins" {
  user_dn    = "CN=admin,OU=Unit,DC=example,DC=com"
  policy_name = "consoleAdmin"
}
