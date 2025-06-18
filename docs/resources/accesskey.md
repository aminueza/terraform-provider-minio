# minio_accesskey Resource

Manages a MinIO access key for a user using the MinIO Admin Go SDK.

## Example Usage

```hcl
# Create a user first
resource "minio_iam_user" "example_user" {
  name = "example-user"
}

# Create an access key with default settings (auto-generated keys, enabled)
resource "minio_accesskey" "example" {
  user = minio_iam_user.example_user.name
}

# Create an access key with custom credentials
resource "minio_accesskey" "custom_key" {
  user       = minio_iam_user.example_user.name
  access_key = "MINIO_ACCESS_KEY"  # Must be 8-20 characters
  secret_key = "mySuperSecretKey"  # Must be at least 8 characters
  status     = "enabled"
}

# Create a disabled access key
resource "minio_accesskey" "disabled_key" {
  user   = minio_iam_user.example_user.name
  status = "disabled"
}
```

## Argument Reference

- `user` (Required) - The MinIO user for whom the access key is managed.
- `access_key` (Optional) - The access key value. If omitted, MinIO generates one. Must be 8-20 characters when specified.
- `secret_key` (Optional) - The secret key value. If omitted, MinIO generates one. Must be at least 8 characters when specified.
- `status` (Optional) - The status of the access key (`enabled` or `disabled`). Defaults to `enabled`.

## Timeouts

`minio_accesskey` provides the following configuration options for timeouts:

- `create` - (Default 5 minutes) How long to wait for an access key to be created.
- `read` - (Default 2 minutes) How long to wait for an access key to be read.
- `update` - (Default 5 minutes) How long to wait for an access key to be updated.
- `delete` - (Default 5 minutes) How long to wait for an access key to be deleted.

## Attributes Reference

- `id` - The access key ID.
- `access_key` - The access key.
- `secret_key` - The secret key.
- `status` - The status of the access key.

## Import

Access keys can be imported using the access key ID. Note that the secret key value cannot be imported and will remain empty in the state.

```sh
terraform import minio_accesskey.example MINIO_ACCESS_KEY
```
