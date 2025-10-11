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

# Create an access key and attach a policy directly (policy name or JSON)
resource "minio_accesskey" "with_policy" {
  user   = minio_iam_user.example_user.name
  access_key = "EXAMPLEKEY1"
  secret_key = "mySuperSecretKey"
  status     = "enabled"
  policy     = file("path/to/policy.json") # or use a policy name or jsonencode block
}

# Output the secret key (only available during resource creation)
output "access_key_secret" {
  value     = minio_accesskey.example.secret_key
  sensitive = true
}
```

## Argument Reference

- `user` (Required) - The MinIO user for whom the access key is managed.
- `access_key` (Optional) - The access key value. If omitted, MinIO generates one. Must be 8-20 characters when specified.
- `secret_key` (Optional) - The secret key value. If omitted, MinIO generates one. Must be at least 8 characters when specified. **Note:** This is a write-only field. The secret key is only available during resource creation and will not be stored in state after the initial apply. Use an output to capture it.
- `status` (Optional) - The status of the access key (`enabled` or `disabled`). Defaults to `enabled`.
- `policy` (Optional) - The policy to attach to the access key. Can be a policy name, a JSON document, or the contents of a file (e.g., `file("path/to/policy.json")`).

## Timeouts

`minio_accesskey` provides the following configuration options for timeouts:

- `create` - (Default 5 minutes) How long to wait for an access key to be created.
- `read` - (Default 2 minutes) How long to wait for an access key to be read.
- `update` - (Default 5 minutes) How long to wait for an access key to be updated.
- `delete` - (Default 5 minutes) How long to wait for an access key to be deleted.

## Attributes Reference

- `id` - The access key ID.
- `access_key` - The access key.
- `secret_key` - The secret key. **Write-only:** Only available immediately after creation. Not persisted in state or retrievable after the initial apply. Capture this value using an output if needed.
- `status` - The status of the access key.

## Import

Access keys can be imported using the access key ID. The secret key cannot be imported or retrieved after creation, as MinIO does not provide an API to read service account secrets.

```sh
terraform import minio_accesskey.example MINIO_ACCESS_KEY
```

## Security Note

The `secret_key` is intentionally designed as write-only to avoid persisting sensitive credentials in Terraform state files. It is only available during the initial `terraform apply` that creates the resource. If you need to store the secret for application use, capture it via an output and securely store it in a secrets management system (e.g., HashiCorp Vault, AWS Secrets Manager).
