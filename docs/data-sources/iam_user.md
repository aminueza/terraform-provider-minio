# minio_iam_user (Data Source)

Reads information about a specific MinIO IAM user.

## Example Usage

```hcl
data "minio_iam_user" "alice" {
  name = "alice"
}

output "alice_status" {
  value = data.minio_iam_user.alice.status
}