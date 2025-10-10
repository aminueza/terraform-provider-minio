# minio_iam_users (Data Source)

Lists MinIO IAM users with optional filtering by name prefix and status.

## Example Usage

### List all enabled users

```hcl
data "minio_iam_users" "all_enabled" {
  status = "enabled"
}

output "enabled_user_count" {
  value = length(data.minio_iam_users.all_enabled.users)
}
```

### Filter users by prefix

```hcl
data "minio_iam_users" "app_users" {
  name_prefix = "app-"
  status      = "enabled"
}

output "app_users" {
  value = [for u in data.minio_iam_users.app_users.users : u.name]
}
```

### List all users (enabled and disabled)

```hcl
data "minio_iam_users" "all" {
  status = "all"
}
```

## Argument Reference

* `name_prefix` - (Optional) Filter users by name prefix. Only users whose names start with this prefix will be returned.
* `status` - (Optional) Filter users by status. Valid values: `enabled`, `disabled`, `all`. Defaults to `enabled`.

## Attribute Reference

* `id` - Unique identifier for this data source.
* `users` - List of user objects. Each user has the following attributes:
  * `name` - The name of the user.
  * `status` - The status of the user (`enabled` or `disabled`).
  * `policy_names` - List of policy names attached to the user (currently returns empty list).
  * `member_of_groups` - List of groups the user belongs to (currently returns empty list).
