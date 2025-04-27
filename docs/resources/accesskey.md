# minio_accesskey Resource

Manages a MinIO access key for a user via the MinIO `mc` CLI.

## Example Usage

```hcl
resource "minio_accesskey" "example" {
  user       = "myuser"
  access_key = "AKIAEXAMPLEKEY"
  secret_key = "mySuperSecretKey"
  status     = "enabled" # or "disabled"
}
```

## Argument Reference

- `user` (Required) - The MinIO user for whom the access key is managed.
- `access_key` (Optional) - The access key value. If omitted, MinIO generates one.
- `secret_key` (Optional) - The secret key value. If omitted, MinIO generates one.
- `status` (Optional) - The status of the access key (`enabled` or `disabled`). Defaults to `enabled`.

## Attributes Reference

- `id` - The access key ID.
- `access_key` - The access key.
- `secret_key` - The secret key.
- `status` - The status of the access key.

## Requirements

- The `mc` CLI must be installed and available in your `$PATH`.
- The `myminio` alias must be configured in `mc` to point to your MinIO server.

Example alias setup:

```sh
mc alias set myminio http://localhost:9000 minioadmin minioadmin
```

## Caveats
- This resource shells out to the `mc` CLI, so the provider host must have access to the CLI and proper permissions.
- Output parsing is best-effort; if MinIO changes CLI output format, parsing may break.
