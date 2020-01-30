# IAM GROUP POLICY ATTACHMENT

Manages IAM Group policy attachment for IAM Users.

## Example of usage

```hcl

resource "minio_iam_group" "developer" {
  name = "developer"
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

resource "minio_iam_group_policy_attachment" "developer" {
  group_name      = "${minio_iam_group.group.name}"
  policy_name = "${minio_iam_policy.test_policy.id}"
}

output "minio_name" {
  value = "${minio_iam_group_policy_attachment.developer.id}"
}

output "minio_users" {
  value = "${minio_iam_group_policy_attachment.developer.group_name}"
}

output "minio_group" {
  value = "${minio_iam_group_policy_attachment.developer.policy_name}"
}
```

## Argument Reference

The following arguments are supported:

* **policy_name** - (Required, Forces new resource) The policy document. This is a JSON formatted string based on AWS policies.
* **group_name** - (Required, Forces new resource) The IAM Group name to attach with a group

## Output

The following outputs are supported:

* **id** - (Optional) The name to identify the group policy attachment.
* **policy_name** - (Optional) The policy attached to group.
* **group_name** - (Optional) The IAM Group name.
