# IAM GROUP

Manages IAM Group for IAM Users.

## Usage

```hcl
resource "minio_iam_group" "developer" {
  name = "developer"
}

output "minio_user_group" {
  value = minio_iam_group.developer.group_name
}
```

### Resource

| Argument | Constraint | Description |
| :---: | :---: | --- |
| `name` | Required | The group's name. The name must consist of upper and lowercase alphanumeric characters with no spaces. You can also include any of the following characters: `=,.@-_.`. Group names are not distinguished by case. For example, you cannot create group named both "ADMINS" and "admins". |
| `force_destroy` | Optional, `false` by default | When destroying this group, destroy even if it has users and polices attached. Without `force_destroy` a group will fail to be destroyed. |
| `disable_group` | Optional, `false` by default | Disable a group. If the group is disabled and you add the flag equals to false, it enables a group back. |

### Output

| Attribute | Constraint | Description |
| :---: | :---: | --- |
| `id` | Optional | Returns the group id. It's same as group name. |
| `group_name` | Optional | Returns the group name. It's same as group id. |
