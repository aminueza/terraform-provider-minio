resource "minio_iam_group" "developer" {
  name = "developer"
}

resource "minio_iam_group_policy" "test_policy" {
  name = "state-terraform-s3"
  group = "${minio_iam_group.developer.id}"
  policy    = <<EOF
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

output "minio_name" {
  value = "${minio_iam_group_membership.developer.id}"
}

output "minio_policy" {
  value = "${minio_iam_group_membership.developer.policy}"
}

output "minio_group" {
  value = "${minio_iam_group_membership.developer.group}"
}