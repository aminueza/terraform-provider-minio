# IAM USER POLICY ATTACHMENT

Manages IAM policy attachment for IAM Users.

## Usage

```go
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
```

### Resource

|   Argument    |          Constraint           | Description                                                                 |
| :-----------: | :---------------------------: | --------------------------------------------------------------------------- |
| `policy_name` | Required, forces new resource | The policy document. This is a JSON formatted string based on AWS policies. |
|  `user_name`  | Required, forces new resource | The IAM User name to be attached with a policy                              |

### Output

|   Attribute   | Constraint | Description                                 |
| :-----------: | :--------: | ------------------------------------------- |
|     `id`      |  Optional  | The name to identify the policy attachment. |
| `policy_name` |  Optional  | The policy name.                            |
|  `user_name`  |  Optional  | The IAM User name.                          |

## Import

IAM user policy attachments can be imported using the user name and policy arn separated by `/`.

```sh
terraform import aws_iam_user_policy_attachment.test-attach test-user/test-policy
```
