## IAM GROUP

Provides an IAM group.

## Example of usage

```hcl
resource "minio_iam_group" "developer" {
  name = "developer"
}

output "minio_user_group" {
  value = "${minio_iam_group.developer.group_name"
}
```

## Argument Reference

The following arguments are supported:

* **name** - (Required) The group's name. The name must consist of upper and lowercase alphanumeric characters with no spaces. You can also include any of the following characters: =,.@-_.. Group names are not distinguished by case. For example, you cannot create group named both "ADMINS" and "admins".
* **force_destroy** - (Optional, default false) When destroying this group, destroy even if it has users and polices attached. Without force_destroy a group will fail to be destroyed.
* **disable_group** - (Optional, default false) Disable a group. If the group is disabled and you add the flag equals to false, it enables a group back.

## Output

The following outputs are supported:

* **id** - (Optional) Returns a group's id. It's same of group name.
* **group_name** - (Optional) Returns a group's name. Same of group's id.
