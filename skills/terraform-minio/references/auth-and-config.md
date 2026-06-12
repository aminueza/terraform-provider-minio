# MinIO provider — authentication & configuration

Full provider configuration for `aminueza/minio`. The provider **source is
`aminueza/minio`**. `minio_server` is a bare `host:port` (no scheme); TLS is
controlled by `minio_ssl`, not by an `https://` prefix.

## All provider arguments

| Argument | Req? | Default | Env var | Notes |
|---|---|---|---|---|
| `minio_server` | **yes** | — | `MINIO_ENDPOINT` | `host:port`. |
| `minio_user` | no | — | `MINIO_USER` | access key. Conflicts with `minio_access_key`. |
| `minio_password` | no | — | `MINIO_PASSWORD` | secret key (sensitive). Conflicts with `minio_secret_key`. |
| `minio_access_key` | no | — | `MINIO_ACCESS_KEY` | **deprecated** → use `minio_user`. |
| `minio_secret_key` | no | — | `MINIO_SECRET_KEY` | **deprecated** → use `minio_password`. |
| `minio_session_token` | no | `""` | `MINIO_SESSION_TOKEN` | temporary STS credentials. |
| `minio_region` | no | `us-east-1` | — | signing region; **must match** S3-compat backends. |
| `minio_api_version` | no | `v4` | — | only `v2` or `v4`. |
| `minio_ssl` | no | `false` | `MINIO_ENABLE_HTTPS` | enable TLS. |
| `minio_insecure` | no | `false` | `MINIO_INSECURE` | skip TLS cert verification. |
| `minio_cacert_file` | no | — | `MINIO_CACERT_FILE` | CA cert path. |
| `minio_cert_file` | no | — | `MINIO_CERT_FILE` | client cert (mTLS). |
| `minio_key_file` | no | — | `MINIO_KEY_FILE` | client key (mTLS, sensitive). |
| `minio_debug` | no | `false` | `MINIO_DEBUG` | request debug logging. |
| `skip_bucket_tagging` | no | `false` | `MINIO_SKIP_BUCKET_TAGGING` | legacy-compat flag. |
| `s3_compat_mode` | no | `false` | `MINIO_S3_COMPAT_MODE` | for non-MinIO S3 backends (see below). |
| `minio_edition` | no | auto-detect | `MINIO_EDITION` | force e.g. `AIStor`. |
| `request_timeout_seconds` | no | `30` | `MINIO_REQUEST_TIMEOUT_SECONDS` | per-request timeout. |
| `max_retries` | no | `6` | `MINIO_MAX_RETRIES` | retry attempts. |
| `retry_delay_ms` | no | `1000` | `MINIO_RETRY_DELAY_MS` | backoff base. |
| `assume_role` | no | — | (sub-args) | STS AssumeRole block (max 1). |
| `assume_role_with_web_identity` | no | — | (sub-args) | OIDC web-identity block (max 1). |

**Auth precedence:** explicit provider arguments override environment variables.
Canonical credential pair is `minio_user` / `minio_password`; do not also set the
deprecated `minio_access_key` / `minio_secret_key` (they conflict).

## STS: `assume_role`

```hcl
provider "minio" {
  minio_server = "minio.example.com:9000"
  minio_ssl    = true
  assume_role {
    role_arn         = "arn:aws:iam::000000000000:role/my-role"  # env MINIO_ASSUME_ROLE_ARN
    session_name     = "terraform"                                # default "terraform"
    duration_seconds = 3600                                       # default 3600
    policy           = jsonencode({ ... })                        # optional inline session policy
    external_id      = "..."                                      # optional
  }
}
```

## OIDC: `assume_role_with_web_identity`

For CI (GitHub Actions, GitLab CI) or Kubernetes service-account tokens:
```hcl
provider "minio" {
  minio_server = "minio.example.com:9000"
  minio_ssl    = true
  assume_role_with_web_identity {
    web_identity_token_file = "/var/run/secrets/token"  # env MINIO_WEB_IDENTITY_TOKEN_FILE
    # web_identity_token    = var.oidc_token            # env MINIO_WEB_IDENTITY_TOKEN
    duration_seconds        = 3600
  }
}
```

## mTLS

```hcl
provider "minio" {
  minio_server      = "minio.example.com:9000"
  minio_ssl         = true
  minio_cacert_file = "/etc/minio/ca.crt"
  minio_cert_file   = "/etc/minio/client.crt"
  minio_key_file    = "/etc/minio/client.key"
}
```
Use `minio_insecure = true` only for self-signed certs in non-production.

## S3-compatible (non-MinIO) backends

Set `s3_compat_mode = true` for Cloudflare R2, Backblaze B2, DigitalOcean Spaces,
Hetzner Object Storage, Versity Gateway, etc.
```hcl
provider "minio" {
  minio_server   = "<account>.r2.cloudflarestorage.com"
  minio_region   = "auto"          # set to the backend's actual region — mismatches fail signing
  minio_user     = var.access_key
  minio_password = var.secret_key
  minio_ssl      = true
  s3_compat_mode = true
}
```
**Caveats:**
- In compat mode the provider **silently skips** features the backend doesn't
  support — bucket notifications, object lock, CORS, lifecycle. Don't expect
  those `minio_*` resources to take effect.
- MinIO-specific features (IAM users/groups/policies, service accounts, server
  config, site replication, notify targets, audit) require a **real MinIO
  server** and won't work against generic S3.
- Region is the most common failure: many gateways reject a mismatched signing
  region with an opaque `AccessDenied`/signature error.

## Secrets handling (summary)

- Source credentials from env vars (`MINIO_*`) or `TF_VAR_*`-fed variables;
  never commit them in HCL.
- Generated secrets (`minio_iam_service_account.secret_key`,
  `minio_iam_user.secret`) are sensitive and persist in **state** — protect state.
- Prefer write-only secret args (`secret_wo`, `secret_key_wo`) where available
  (Terraform ≥ 1.11) so values never enter state, or `minio_accesskey` whose
  `secret_key` is write-only by design.
- Never print a secret value back to the user; point them to the output/resource.

## Licensing note

The provider is **AGPL-3.0** (from v2.0.0). If that matters for the user's
distribution model, flag it — it only affects the provider binary's license, not
the data or buckets it manages.
