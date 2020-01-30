# IAM POLICY

Provides an IAM policy.

## Example of usage

```hcl

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
  value = "${minio_iam_policy.test_policy.policy}"
}
```

## Argument Reference

The following arguments are supported:

* **name** - (Optional, Forces new resource) The name of the policy. If omitted, Terraform will assign a random, unique name.
* **policy** - (Required) The policy document. This is a JSON formatted string based on AWS policies.
* **name_prefix** - (Optional, Forces new resource) Creates an unique name beginning with the specified prefix. Conflicts with name.

## Output

The following outputs are supported:

* **id** - (Optional) Returns a policy's id. It's same of policy name.
* **name** - (Optional) Returns a policy's name. Same of policy's id.
* **policy** - (Optional) Returns a policy's json string.
