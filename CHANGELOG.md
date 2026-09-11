# Changelog

All notable, user-visible changes to this provider are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

History older than 3.39.0 lives in the
[GitHub Releases](https://github.com/aminueza/terraform-provider-minio/releases) pages.

## [Unreleased]

### Added

- `minio_sts_credentials` ephemeral resource: mints temporary credentials
  through STS AssumeRole without writing them to Terraform state. Use it to
  configure another provider instance or to feed a write-only attribute.
  `duration_seconds` accepts 3600 or more, since the MinIO client library
  replaces lower values with 3600. The resource requires a provider configured
  with static credentials: it refuses to run when the provider uses
  `assume_role`, `assume_role_with_web_identity` or a session token, because it
  would otherwise issue credentials outside the scope those establish, and
  MinIO rejects AssumeRole made with temporary credentials. Every Terraform
  operation opens the resource and mints a new server-side session that lives
  until it expires. Requires Terraform 1.10 or later
  ([#1146](https://github.com/aminueza/terraform-provider-minio/issues/1146)).

### Changed

- The provider is now served through `terraform-plugin-mux`, combining the
  existing SDKv2 resources and data sources with a plugin-framework half that
  serves ephemeral resources. Every existing resource and data source is
  unchanged; the provider now speaks protocol 6, which Terraform 1.0 and later
  already negotiate.

### Fixed

- `minio_sts_credentials` now uses the provider TLS settings. The request went
  out on the default HTTP client, so `minio_insecure`, `minio_cacert_file`,
  `minio_cert_file`, `minio_key_file` and `request_timeout_seconds` were all
  ignored. Against a server with a private certificate authority the resource
  failed with `x509: certificate signed by unknown authority` while every other
  resource worked. The request now also stops when Terraform is interrupted
  ([#1151](https://github.com/aminueza/terraform-provider-minio/issues/1151)).
- `MINIO_REQUEST_TIMEOUT_SECONDS`, `MINIO_MAX_RETRIES` and `MINIO_RETRY_DELAY_MS`
  now set `request_timeout_seconds`, `max_retries` and `retry_delay_ms`. The
  three attributes declared a static default beside the environment lookup, and
  the static default won, so the variables never took effect. A deployment that
  already sets one of them gets the value it asked for after this upgrade, where
  before it got 30, 6 and 1000
  ([#1149](https://github.com/aminueza/terraform-provider-minio/issues/1149)).
- A value that is not a whole number now stops every command that loads the
  provider, `destroy` included, instead of being ignored. The error names the
  variable: `MINIO_MAX_RETRIES must be a whole number, got "abc"`.
- `request_timeout_seconds` of 0 or less falls back to 30. It used to reach the
  HTTP transport, where every call then failed with a misleading `i/o timeout`.
  `minio_health_status` fell back to 10 seconds in that case and now uses 30 too.
- `retry_delay_ms` is not the base delay between retries, as its description
  said. It caps the wait at 20 times its value in milliseconds, and the wait
  itself is random and doubles with each attempt. The description and the
  documentation now say so. The retry behaviour is unchanged.

## [3.41.1] - 2026-09-03

### Changed

- Bumped `github.com/minio/madmin-go/v4` from 4.10.3 to 4.10.5
  ([#1130](https://github.com/aminueza/terraform-provider-minio/pull/1130)).
- Bumped `golang.org/x/crypto` from 0.55.0 to 0.56.0
  ([#1137](https://github.com/aminueza/terraform-provider-minio/pull/1137)).
- Bumped `google.golang.org/grpc` from 1.82.1 to 1.83.1
  ([#1136](https://github.com/aminueza/terraform-provider-minio/pull/1136)).

### Fixed

- Made `TestAccMinioIAMUser_import` trailing plans deterministic
  ([#1129](https://github.com/aminueza/terraform-provider-minio/pull/1129)).

## [3.41.0] - 2026-08-26

### Changed

- `minio_iam_group`: changing `name` now recreates the group (`ForceNew`).
  MinIO has no rename operation; before this change a rename was silently
  ignored and Terraform state diverged from the server
  ([#1127](https://github.com/aminueza/terraform-provider-minio/pull/1127)).

### Fixed

- `minio_iam_group`: `force_destroy` now actually deletes a group that still has
  members, and no longer fires during ordinary updates (previously an apply that
  changed any other attribute on a group carrying the flag deleted the group and
  failed with `Provider produced inconsistent result after apply`) ([#1113](https://github.com/aminueza/terraform-provider-minio/pull/1113)).
- `minio_iam_group`: deleting a group that still has members now waits for the
  membership to clear and then fails with a clear error pointing at
  `force_destroy`, instead of silently leaving the group behind on the server
  ([#1109](https://github.com/aminueza/terraform-provider-minio/pull/1109)).
- Error messages no longer contain literal printf verbs such as
  `error updating IAM Group %s: %s`; twenty-one messages across IAM group,
  IAM user, service account and bucket policy resources now state the
  operation plainly ([#1121](https://github.com/aminueza/terraform-provider-minio/pull/1121)).
- `minio_config_history` and `minio_license_info` reads against servers that
  do not serve those admin APIs (community MinIO) now return within seconds
  instead of retrying for minutes before reporting the graceful fallback
  ([#1126](https://github.com/aminueza/terraform-provider-minio/pull/1126)).

## [3.40.1] - 2026-08-04

### Fixed

- `minio_s3_bucket_anonymous_access` and `minio_s3_bucket`'s `acl` now write
  the canned anonymous policies `mc` recognizes, so `mc anonymous get` reports
  `download`, `upload` and `public` instead of `custom`. `public-read-write`
  also grants `s3:GetObject`, which it never did before (anonymous `GET`
  returned `403`). Re-apply your configuration to replace the old policies;
  this is a plain `PutBucketPolicy`, with no recreate
  ([#1094](https://github.com/aminueza/terraform-provider-minio/pull/1094)).

### Removed

- The AIStor-specific anonymous policies, which could not be applied on any
  backend. `minio_edition` and `minio_server_info.edition` are unchanged
  ([#1094](https://github.com/aminueza/terraform-provider-minio/pull/1094)).

### Security

- Vulnerability reports now flow exclusively through
  [GitHub Private Vulnerability Reporting](https://github.com/aminueza/terraform-provider-minio/security/advisories/new);
  the issue template that created public "security" issues was removed
  ([#1090](https://github.com/aminueza/terraform-provider-minio/pull/1090),
  [#1091](https://github.com/aminueza/terraform-provider-minio/pull/1091)).

## [3.40.0] - 2026-08-03

### Security

- **`minio_s3_bucket_anonymous_access` with `access_type = "public"` granted
  anonymous callers bucket administration** — `s3:PutBucketPolicy`,
  `s3:DeleteBucketPolicy`, `s3:DeleteBucket` and `s3:CreateBucket` — in
  addition to object read/write. Any unauthenticated client could rewrite the
  bucket policy or delete the bucket. Fixed in
  [#1085](https://github.com/aminueza/terraform-provider-minio/pull/1085).

  **Upgrading the provider alone is not enough.** The corrected policy is only
  written when the resource is re-applied: after upgrading, run
  `terraform apply` against every workspace that manages an
  `access_type = "public"` bucket. The rewrite is a plain `PutBucketPolicy` —
  no recreate, and no change to who can read or write objects. To verify,
  `mc anonymous get-json <alias>/<bucket>` should list no `s3:*Bucket*` actions
  beyond `s3:GetBucketLocation`, `s3:ListBucket` and
  `s3:ListBucketMultipartUploads`. Only buckets using `access_type = "public"`
  are affected; `public-read`, `public-write`, `public-read-write` and custom
  policies are not.

## [3.39.0] - 2026-08-02

### Added

- Support for MinIO servers that require **admin API v4**, by migrating from
  `madmin-go/v3` to `madmin-go/v4`. Servers that rejected the provider with
  `Server expects client requests with 'admin' API version 'v4'` now work
  ([#1074](https://github.com/aminueza/terraform-provider-minio/pull/1074)).

### Changed

- Against servers that only speak admin API v3, each admin request first tries
  v4 and falls back to v3 after a `426 Upgrade Required` response. Deployments
  on older MinIO servers that notice added latency per admin operation can set
  the `MADMIN_API_VERSION=v3` environment variable on the machine running
  Terraform to skip the fallback entirely.

[Unreleased]: https://github.com/aminueza/terraform-provider-minio/compare/v3.41.1...HEAD
[3.41.1]: https://github.com/aminueza/terraform-provider-minio/compare/v3.41.0...v3.41.1
[3.41.0]: https://github.com/aminueza/terraform-provider-minio/compare/v3.40.1...v3.41.0
[3.40.1]: https://github.com/aminueza/terraform-provider-minio/compare/v3.40.0...v3.40.1
[3.40.0]: https://github.com/aminueza/terraform-provider-minio/compare/v3.39.0...v3.40.0
[3.39.0]: https://github.com/aminueza/terraform-provider-minio/compare/v3.38.6...v3.39.0
