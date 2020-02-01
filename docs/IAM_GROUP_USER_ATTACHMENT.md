# IAM GROUP USER ATTACHMENT

Manages IAM Group Membership for a single IAM User.

## Example of usage

```hcl

resource "minio_iam_group" "developer" {
  name = "developer"
}
resource "minio_iam_user" "user_one" {
  name = "test-user"
}

resource "minio_iam_group_user_attachment" "developer" {
  group_name = minio_iam_group.group.name
  user_name  = minio_iam_user.user_one.name
}

output "minio_name" {
  value = minio_iam_group_user_attachment.developer.id
}

output "minio_users" {
  value = minio_iam_group_user_attachment.developer.group_name
}

output "minio_group" {
  value = minio_iam_group_user_attachment.developer.user_name
}
```

## Argument Reference

The following arguments are supported:

* **group_name** - (Required, Forces new resource) The IAM Group name.
* **user_name** - (Required, Forces new resource) The IAM User name, for adding.

## Output

The following outputs are supported:

* **id** - (Optional) The name to identify the group user membership.
* **group_name** - (Optional) The IAM Group name.
* **user_name** - (Optional) The Username attached to group.


## Import

IAM Group User attachments can be imported using the group name and user name separated by `/`.

```bash
  $ terraform import minio_iam_group_user_attachment.developer developer/test-user
```
