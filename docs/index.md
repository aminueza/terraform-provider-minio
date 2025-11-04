---
page_title: "Provider: Minio"
description: Manage Minio with Terraform.
---

# Minio Provider

This is a terraform provider plugin for managing [Minio](https://min.io/) S3 buckets and IAM users.

## Example Provider Configuration

```terraform
provider "minio" {
  // required
  minio_server   = "..."
  minio_user     = "..."
  minio_password = "..."

  // optional
  minio_region      = "..."
  minio_api_version = "..."
  minio_ssl         = "..."
}
```

## Authentication

The Minio provider offers the following methods of providing credentials for
authentication, in this order, and explained below:

- Static API key
- Environment variables

### Static API Key

Static credentials can be provided by adding the `minio_server`, `minio_user` and `minio_password` variables in-line in the
Minio provider block:

Usage:

```hcl
provider "minio" {
  minio_server   = "..."
  minio_user     = "..."
  minio_password = "..."
}
```

### Environment variables

You can provide your configuration via the environment variables representing your minio credentials:

```
$ export MINIO_ENDPOINT="http://myendpoint"
$ export MINIO_USER="244tefewg"
$ export MINIO_PASSWORD="xgwgwqqwv"
```

When using this method, you may omit the
minio `provider` block entirely:

```hcl
resource "minio_s3_bucket" "state_terraform_s3" {
  # .....
}
```

## Argument Reference

The following arguments are supported in the `provider` block:

* `minio_server` - (Required) MinIO server endpoint in the format `host:port`. Can be sourced from `MINIO_ENDPOINT`.

* `minio_user` - (Optional) MinIO user (or access key). Can be sourced from `MINIO_USER`. Conflicts with `minio_access_key`.

* `minio_password` - (Optional, Sensitive) MinIO password (or secret key). Can be sourced from `MINIO_PASSWORD`. Conflicts with `minio_secret_key`.

* `minio_access_key` - (Optional) MinIO access key. Deprecated: use `minio_user` instead. Can be sourced from `MINIO_ACCESS_KEY`.

* `minio_secret_key` - (Optional, Sensitive) MinIO secret key. Deprecated: use `minio_password` instead. Can be sourced from `MINIO_SECRET_KEY`.

* `minio_session_token` - (Optional, Sensitive) Session token for temporary credentials. Can be sourced from `MINIO_SESSION_TOKEN`.

* `minio_region` - (Optional) MinIO region (default: `us-east-1`).

* `minio_api_version` - (Optional) MinIO API version (`v2` or `v4`, default: `v4`).

* `minio_ssl` - (Optional) Enable SSL/TLS (default: `false`). Can be sourced from `MINIO_ENABLE_HTTPS`.

* `minio_insecure` - (Optional) Skip SSL certificate verification (default: `false`). Can be sourced from `MINIO_INSECURE`.

* `minio_cacert_file` - (Optional) Path to CA certificate file. Can be sourced from `MINIO_CACERT_FILE`.

* `minio_cert_file` - (Optional) Path to client certificate file. Can be sourced from `MINIO_CERT_FILE`.

* `minio_key_file` - (Optional, Sensitive) Path to client private key file. Can be sourced from `MINIO_KEY_FILE`.
