# IAM GROUP POLICY

Manages IAM Group policies for IAM Users.

## Example of usage

```hcl

resource "minio_iam_group" "developer" {
  name = "developer"
}

resource "minio_iam_policy" "test_policy" {
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
```

## Argument Reference

The following arguments are supported:

* **policy** - (Required) The policy document. This is a JSON formatted string based on AWS policies.
* **name** - (Optional, Forces new resource) The name of the policy. If omitted, Terraform will assign a random, unique name.
* **name_prefix** - (Optional, Forces new resource) Creates an unique name beginning with the specified prefix. Conflicts with name.
* **group** - (Required) The IAM Group name to attach the policy.

## Output

The following outputs are supported:

* **id** - (Optional) Returns a group's policy id. It's same of group policy name.
* **policy** - (Optional) The policy document.
* **name** - (Optional) The name of the policy.
* **group** - (Optional) The IAM Group name to attach the policy.
