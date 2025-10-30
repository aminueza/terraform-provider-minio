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
  user              = minio_iam_user.example_user.name
  access_key        = "MINIO_ACCESS_KEY"  # Must be 8-20 characters
  secret_key        = "mySuperSecretKey"  # Must be at least 8 characters
  secret_key_version = "v1"               # Version identifier for change detection
  status            = "enabled"
}

# Create a disabled access key
resource "minio_accesskey" "disabled_key" {
  user   = minio_iam_user.example_user.name
  status = "disabled"
}

# Create an access key and attach a policy directly (policy name or JSON)
resource "minio_accesskey" "with_policy" {
  user               = minio_iam_user.example_user.name
  access_key         = "EXAMPLEKEY1"
  secret_key         = "mySuperSecretKey"
  secret_key_version = "v1"
  status             = "enabled"
  policy             = file("path/to/policy.json") # or use a policy name or jsonencode block
}

# Rotate secret key by changing the version
resource "minio_accesskey" "rotated_key" {
  user               = minio_iam_user.example_user.name
  access_key         = "EXAMPLEKEY1"
  secret_key         = "myNewSuperSecretKey"
  secret_key_version = "v2"  # Change this to trigger rotation
  status             = "enabled"
}

# Using with Vault KV V2 - just pass the version number
resource "minio_accesskey" "vault_key" {
  user               = minio_iam_user.example_user.name
  secret_key         = data.vault_kv_secret_v2.minio_secret.data["secret_key"]
  secret_key_version = tostring(data.vault_kv_secret_v2.minio_secret.version)
  status             = "enabled"
}

# Output the access key (secret_key is write-only and never exposed in outputs)
output "access_key_id" {
  value = minio_accesskey.example.access_key
}
```

## Argument Reference

- `user` (Required) - The MinIO user for whom the access key is managed.
- `access_key` (Optional) - The access key value. If omitted, MinIO generates one. Must be 8-20 characters when specified.
- `secret_key` (Optional) - The secret key value. Must be at least 8 characters when specified. **Note:** This is a write-only field and will not be stored in state. When provided, `secret_key_version` must also be specified.
- `secret_key_version` (Optional) - Version identifier for the secret key. Required when `secret_key` is provided. Change this value to trigger a secret key rotation. Can be a hash (e.g., SHA-256), version number, timestamp, Vault version, or any string that changes when the secret changes. This provides flexibility for different secret management approaches.
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
- `secret_key` - The secret key. **Write-only:** Never persisted in state or exposed in outputs.
- `secret_key_version` - The version identifier for the secret key.
- `status` - The status of the access key.

## Import

Access keys can be imported using the access key ID. The secret key cannot be imported or retrieved after creation, as MinIO does not provide an API to read service account secrets. The provider does not expose secret keys at any time.

```sh
terraform import minio_accesskey.example MINIO_ACCESS_KEY
```

## Security Note

The `secret_key` is intentionally designed as write-only to avoid persisting sensitive credentials in Terraform state files. It is never exposed in state or outputs, including at creation time. When you provide a custom secret key, you must also provide a `secret_key_version` to enable change detection without storing the actual secret.

### Secret Key Rotation

To rotate a secret key:
1. Update both `secret_key` and `secret_key_version` in your configuration
2. Run `terraform apply`
3. The new secret will be set, but not stored in state

### Integration with Secret Management Systems

This design is particularly useful for integrating with secret management systems:

**HashiCorp Vault KV V2:**
```hcl
data "vault_kv_secret_v2" "minio_secret" {
  mount = "secret"
  name  = "minio/accesskey"
}

resource "minio_accesskey" "example" {
  user               = minio_iam_user.example_user.name
  secret_key         = data.vault_kv_secret_v2.minio_secret.data["secret_key"]
  secret_key_version = tostring(data.vault_kv_secret_v2.minio_secret.version)
}
```

**Using a hash for version tracking:**
```hcl
resource "minio_accesskey" "example" {
  user               = minio_iam_user.example_user.name
  secret_key         = var.secret_key
  secret_key_version = sha256(var.secret_key)
}
```
