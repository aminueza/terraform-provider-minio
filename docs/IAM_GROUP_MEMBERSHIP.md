# IAM GROUP MEMBERSHIP

Manages IAM Group membership for IAM Users.

## Usage

```hcl
resource "minio_iam_group" "developer" {
  name = "developer"
}

resource "minio_iam_user" "user_one" {
  name = "test-user"
}

resource "minio_iam_user" "user_two" {
  name = "test-user-two"
}

resource "minio_iam_group_membership" "developer" {
    name = "tf-testing-group-membership"

  users = [
    "${minio_iam_user.user_one.name}",
    "${minio_iam_user.user_two.name}",
  ]

  group = minio_iam_group.developer.name
}

output "minio_name" {
  value = "${minio_iam_group_membership.developer.id}"
}

output "minio_users" {
  value = "${minio_iam_group_membership.developer.users}"
}

output "minio_group" {
  value = "${minio_iam_group_membership.developer.group}"
}
```

### Resource

| Argument | Constraint | Description                                           |
| :------: | :--------: | ----------------------------------------------------- |
|  `name`  |  Required  | The name to identify the Group Membership.            |
| `users`  |  Required  | A list of IAM User names to associate with the Group. |
| `group`  |  Required  | The IAM Group name to attach the list of `users` to.  |

### Output

| Attribute | Constraint | Description                                       |
| :-------: | :--------: | ------------------------------------------------- |
|   `id`    |  Optional  | The name to identify the Group Membership.        |
|  `users`  |  Optional  | A list of IAM User names associated to the Group. |
|  `group`  |  Optional  | The IAM Group name.                               |
