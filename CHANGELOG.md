# Changelog

All notable, user-visible changes to this provider are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

History older than 3.39.0 lives in the
[GitHub Releases](https://github.com/aminueza/terraform-provider-minio/releases) pages.

## [Unreleased]

### Fixed

- `minio_iam_group`: `force_destroy` now actually deletes a group that still has
  members, and no longer fires during ordinary updates (previously an apply that
  changed any other attribute on a group carrying the flag deleted the group and
  failed with `Provider produced inconsistent result after apply`) ([#1113](https://github.com/aminueza/terraform-provider-minio/pull/1113)).
- `minio_iam_group`: deleting a group that still has members now waits for the
  membership to clear and then fails with a clear error pointing at
  `force_destroy`, instead of silently leaving the group behind on the server
  ([#1109](https://github.com/aminueza/terraform-provider-minio/pull/1109)).

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

[Unreleased]: https://github.com/aminueza/terraform-provider-minio/compare/v3.40.1...HEAD
[3.40.1]: https://github.com/aminueza/terraform-provider-minio/compare/v3.40.0...v3.40.1
[3.40.0]: https://github.com/aminueza/terraform-provider-minio/compare/v3.39.0...v3.40.0
[3.39.0]: https://github.com/aminueza/terraform-provider-minio/compare/v3.38.6...v3.39.0
