# Changelog

All notable, user-visible changes to this provider are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

History older than 3.39.0 lives in the
[GitHub Releases](https://github.com/aminueza/terraform-provider-minio/releases) pages.

## [Unreleased]

### Fixed

- Removing `comment` or `queue_dir` from a `minio_notify_*` resource clears it
  on the server. MinIO merges a config update into the target's current
  settings, so the old value stayed and every plan showed the removal again.
  Read also picks up a comment or queue directory cleared outside Terraform.
- The `minio_ilm_policy` data source reports errors from reading the bucket
  lifecycle (missing bucket, permissions, server error) instead of returning an
  empty `rules` list, which read as "this bucket has no lifecycle rules". A
  bucket without lifecycle rules still reads as empty. A key without
  `s3:GetLifecycleConfiguration` now fails the read. With `s3_compat_mode`, a
  backend that does not implement bucket lifecycle gives a warning.
  ([#1282](https://github.com/aminueza/terraform-provider-minio/issues/1282)).
- `minio_ilm_policy` removes the bucket's lifecycle configuration when an
  update leaves only abort-only rules (`abort_incomplete_multipart_upload`
  without another action). The old rules, for example an `expiration`, stayed
  on the bucket and every plan showed the same change again
  ([#1286](https://github.com/aminueza/terraform-provider-minio/issues/1286)).

### Security

- Bumped the Go toolchain from go1.26.10 to go1.26.11, which fixes
  [GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859): an HTTP/1 server
  using `MaxBytesReader` could crash on a header race in `net/http`. The
  provider only makes HTTP client calls, so it was not exposed, but release
  binaries are built with this toolchain and the security scan blocks every
  pull request until it moves.

## [3.46.3] - 2026-11-10

### Fixed

- `minio_s3_bucket_server_side_encryption` reports errors from reading the
  bucket encryption (network, permissions, server error) instead of dropping
  the resource from state, which made the next plan write the configuration
  again. It is still removed from state when the encryption configuration or
  the bucket was deleted outside Terraform, and destroying it no longer fails
  in that case.
- `minio_s3_bucket_server_side_encryption` no longer shows a diff on every
  plan when `encryption_type = "AES256"` and `kms_key_id` is set. The key is
  ignored for SSE-S3 and MinIO does not return it, so Read always stored an
  empty value.
- The `minio_s3_bucket_encryption` data source reports errors from reading
  the bucket encryption (missing bucket, permissions, server error) instead of
  returning an empty `encryption_type`, which read as "this bucket is not
  encrypted". A bucket without default encryption still reads as empty. A key
  without `s3:GetEncryptionConfiguration` now fails the read. With
  `s3_compat_mode`, a backend that does not implement bucket encryption gives a
  warning.
- `minio_ilm_policy` reports errors from reading the lifecycle configuration
  (network, permissions, server error) instead of dropping the policy from
  state, which made the next plan write it again. It is still removed from
  state when the lifecycle configuration or the bucket was deleted outside
  Terraform, and destroying it no longer fails when the bucket is gone.
- `minio_s3_object_retention.retain_until_date` and `minio_s3_object.expires`
  no longer show a diff on every plan when the timestamp is written with an
  offset other than UTC (for example `2030-01-01T00:00:00-03:00`). MinIO
  returns both in UTC, so the values are now compared as instants.
- `minio_iam_group_policy` reports errors from reading the canned policy
  (network, permissions, server error) instead of dropping it from state, and
  destroy reports them instead of succeeding without deleting the policy. It
  is still removed from state when the policy was deleted outside Terraform.
- `minio_ilm_policy` reports errors from updating its rules, such as a denied
  lifecycle write. Update ignored them, so the apply succeeded, and the next
  plan showed the same change again or planned to create the policy.

## [3.46.2] - 2026-11-07

### Fixed

- `minio_s3_object_tags` is removed from state when its object or bucket was
  deleted outside Terraform, and destroying it no longer fails in that case.
  Every plan used to fail with `NoSuchKey` or `NoSuchBucket` until the
  resource was removed from state by hand.
- `minio_kms_key` no longer drops the key from state when reading its status
  fails (network, permissions, server error); the error is reported instead.
  A key deleted outside Terraform is now detected and removed from state,
  where it used to fail every plan with an encryption error.

## [3.46.1] - 2026-11-07

### Fixed

- `minio_s3_bucket_replication` now rejects two rules on the same
  `target.host`, whatever their target bucket, before it creates any remote
  target ([#1258](https://github.com/aminueza/terraform-provider-minio/issues/1258)).
  MinIO allows only one remote target per endpoint for a bucket, so this
  configuration used to fail with `BucketRemoteAlreadyExists` after the first
  remote target was already created.
- `minio_s3_bucket_replication` reports a failed removal of unused remote
  targets as a warning and keeps the resource in state
  ([#1260](https://github.com/aminueza/terraform-provider-minio/issues/1260)).
  The replication configuration is already written at that point, so the next
  plan used to propose creating it again. Read already ignores remote targets
  no rule uses.
- `minio_s3_bucket_replication` can change a rule's `target.bucket` or
  `target.path` while it keeps its `target.host`
  ([#1257](https://github.com/aminueza/terraform-provider-minio/issues/1257)).
  The provider now drops the rule from the configuration and removes the old
  remote target before it creates the new one; this used to fail with
  `BucketRemoteAlreadyExists`. Toggling `target.synchronous` now updates the
  remote target in place, where it used to be ignored or fail the same way. If
  creating the new remote target fails, the error says that replication to
  that host is stopped until an apply succeeds.

## [3.46.0] - 2026-11-03

### Added

- `minio_ilm_policy`: new `all_versions_expiration` block (`days`,
  `delete_marker`) mapping the MinIO AIStor `AllVersionsExpiration` lifecycle
  action. With `delete_marker = true` it purges objects whose latest version is
  a delete marker, delete marker included. MinIO community servers reject this
  action.

### Fixed

- `minio_s3_bucket_replication` no longer pairs a rule with the remote target
  of the rule that used to sit at the same position
  ([#1250](https://github.com/aminueza/terraform-provider-minio/issues/1250)).
  Removing a rule that is not the last one used to send its neighbour's
  credentials to the wrong remote target and fail the apply. A remote target is
  now identified by its endpoint and target bucket, and a rule keeps its ID
  through its remote target, so rules can be removed or reordered.
- Removing a rule from `minio_s3_bucket_replication` no longer fails with
  `This ARN is in use by an existing configuration`: the remote targets of
  removed rules are deleted after the new configuration is written.
- Destroying `minio_s3_bucket_replication` removes its remote targets instead of
  failing with `remote targets are still present on the bucket`.
- Reading `minio_s3_bucket_replication` no longer fails with `inconsistent
  number of remote target and bucket replication rules` when the bucket has a
  remote target that no rule uses
  ([#1255](https://github.com/aminueza/terraform-provider-minio/issues/1255)).
  The provider logs a warning, and the next apply that changes the resource, or
  destroying it, removes the target.
- Two `minio_s3_bucket_replication` rules that replicate to the same bucket on
  the same endpoint are now rejected before any remote target is created or
  updated.

### Security

- Bumped `golang.org/x/net` from 0.60.0 to 0.62.0 for
  [GO-2026-6860](https://pkg.go.dev/vuln/GO-2026-6860) and
  [GO-2026-6866](https://pkg.go.dev/vuln/GO-2026-6866) in `x/net/html`. The
  provider does not import `x/net/html`, so released binaries were not
  affected; the bump clears the advisories from the dependency scan.

## [3.45.1] - 2026-10-26

### Changed

- Bumped `github.com/minio/minio-go/v7` from 7.3.0 to 7.4.1
  ([#1232](https://github.com/aminueza/terraform-provider-minio/pull/1232)).
  It no longer replaces an STS `DurationSeconds` below 3600 with 3600.
  `minio_sts_credentials` keeps rejecting values below 3600, and the
  `duration_seconds` description no longer gives the old client behaviour as
  the reason.
- Bumped `google.golang.org/grpc` from 1.83.2 to 1.84.0
  ([#1231](https://github.com/aminueza/terraform-provider-minio/pull/1231)).
- Bumped `google.golang.org/protobuf` from 1.36.11 to 1.36.12
  ([#1233](https://github.com/aminueza/terraform-provider-minio/pull/1233)).

### Security

- Bumped the Go toolchain from go1.26.9 to go1.26.10, which fixes
  [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809): `crypto/x509`
  trusted compromised intermediate certificates. Release binaries are built
  with this toolchain, so the fix reaches the TLS connections the provider
  opens to MinIO.

## [3.45.0] - 2026-10-13

### Added

- `minio_region` can now be set from the `MINIO_REGION` environment variable,
  like the other provider settings. Backends that check the signing region,
  such as Backblaze B2 and Hetzner Object Storage, could only be configured
  through the provider block before
  ([#1174](https://github.com/aminueza/terraform-provider-minio/issues/1174)).

### Changed

- Bumped `golang.org/x/sync` from 0.23.0 to 0.24.0
  ([#1222](https://github.com/aminueza/terraform-provider-minio/pull/1222)).
- Bumped the Go toolchain to go1.26.9
  ([#1221](https://github.com/aminueza/terraform-provider-minio/pull/1221)).

### Fixed

- Destroying a `minio_iam_group`, including the old group of a replacement
  after a rename, now confirms that MinIO no longer lists the group and removes
  it again while it does. CI saw a group answer a successful removal and stay
  listed for more than six seconds, so Terraform recorded it as destroyed while
  it still existed. A group still listed after six attempts now fails the
  destroy instead of being dropped from state
  ([#1218](https://github.com/aminueza/terraform-provider-minio/pull/1218)).

### Security

- Bumped `golang.org/x/net` from 0.58.0 to 0.60.0, which fixes four HTTP/2
  vulnerabilities that `govulncheck` finds reachable from the provider through
  the plugin server: an HPACK encoder race that crashes the server
  ([GO-2026-6617](https://pkg.go.dev/vuln/GO-2026-6617)), a double flow-control
  refund ([GO-2026-6612](https://pkg.go.dev/vuln/GO-2026-6612)), CPU exhaustion
  from repeated window changes
  ([GO-2026-6611](https://pkg.go.dev/vuln/GO-2026-6611)) and memory exhaustion
  from trailer headers ([GO-2026-6603](https://pkg.go.dev/vuln/GO-2026-6603)).
  `golang.org/x/crypto`, `x/mod`, `x/sys` and `x/text` move to the versions it
  requires.

## [3.44.0] - 2026-10-02

### Changed

- With `s3_compat_mode = true`, a read the backend cannot answer now leaves the
  resource in state with the attributes the last write stored, where it emptied
  them. Emptying them reproduced the perpetual diff of
  [#1173](https://github.com/aminueza/terraform-provider-minio/issues/1173),
  regressed CORS on a backend without it, and reset `ForceNew` identity
  attributes such as `name`, `bucket` and `endpoint`, which forced a replacement
  and then failed the delete. The warning naming the feature is unchanged
  ([#1198](https://github.com/aminueza/terraform-provider-minio/pull/1198)).
- With `s3_compat_mode = true`, a 405 `MethodNotAllowed` response is treated as
  a missing feature on the bucket sub-resources and on writes, but no longer on
  the object-level reads of `minio_s3_object`, `minio_s3_object_tags`,
  `minio_s3_object_legal_hold` and `minio_s3_object_retention`. A GET or HEAD
  whose `versionId` is a delete marker is answered with 405, so there it is an
  answer about the object rather than an absent feature, and absorbing it
  reported an object as readable when it is gone. A 501 `NotImplemented`
  response is still treated as a missing feature on every resource, and every
  such response is an error with the flag off
  ([#1198](https://github.com/aminueza/terraform-provider-minio/pull/1198)).
- The `s3_compat_mode` provider attribute now describes those two rules, and
  states that the admin API resources `minio_ilm_tier` and
  `minio_s3_bucket_quota` are recognised by error code alone, because the admin
  client reports no HTTP status
  ([#1198](https://github.com/aminueza/terraform-provider-minio/pull/1198)).
- Bumped `github.com/hashicorp/awspolicyequivalence` from 1.7.0 to 1.8.0. The
  release rewrites `interface{}` as `any`, requires Go 1.26 and moves its
  indirect `aws-sdk-go-v2` dependency to 1.47.0; the policy comparison the
  provider uses to suppress diffs between equivalent policies is unchanged
  ([#1182](https://github.com/aminueza/terraform-provider-minio/pull/1182)).
- Bumped `github.com/dustin/go-humanize` from 1.0.1 to 1.1.0. `Bytes()` now
  rounds once at the target precision instead of twice, which changes the
  rendering of values at a `.x5` boundary such as `10.5GB`, shown as `11 GB`
  where 1.0.1 showed `10 GB`. The provider renders `bandwidth_limit` on
  `minio_s3_bucket_replication` with it, see
  [#1187](https://github.com/aminueza/terraform-provider-minio/issues/1187)
  ([#1181](https://github.com/aminueza/terraform-provider-minio/pull/1181)).
- `minio_s3_bucket_replication` now stores `bandwidth_limit` in state as the
  exact number of bytes per second the server reports, for example
  `10500000000`, where it used to store a rounded rendering such as `10 GB`.
  The configuration keeps accepting suffixes such as `100M` or `1.5G`. A state
  written by an older release shows a one-time diff on the next refresh, and no
  request goes to the server for it
  ([#1187](https://github.com/aminueza/terraform-provider-minio/issues/1187)).

### Fixed

- Deleting an empty bucket with `force_destroy` now works on backends that
  do not support versioned object listing (e.g. Garage), by falling back to
  a plain listing when the versioned call returns `NotImplemented` or `501`
  ([#1174](https://github.com/aminueza/terraform-provider-minio/issues/1174)).
- `minio_s3_bucket_anonymous_access` now sets its id only after the policy has
  been written. It set the id first, so a backend that rejected the write left a
  resource in state for one that was never created, and the next plan read it
  back
  ([#1173](https://github.com/aminueza/terraform-provider-minio/issues/1173)).
- `minio_s3_bucket_replication` no longer ignores a change to `bandwidth_limit`
  that rounds to the same two-digit rendering as the current value, such as
  `1000MB` to `1040MB`. The diff suppression compared rendered strings, so the
  change never reached the server. It now compares the parsed byte counts, so
  the result no longer depends on how `go-humanize` rounds a rendering
  ([#1187](https://github.com/aminueza/terraform-provider-minio/issues/1187)).
- `minio_s3_bucket_lifecycle` and `minio_ilm_policy` now treat a `204 No Content`
  answer to `GetBucketLifecycle` as "no lifecycle configuration", the same as
  `NoSuchLifecycleConfiguration`. Garage answers 204 on a bucket without one,
  and the provider failed every create there with
  `reading existing lifecycle: 204 No Content`, so no rule could be managed on
  Garage, including the Expiration and AbortIncompleteMultipartUpload rules it
  supports. The check now reads the typed error instead of its message text
  ([#1205](https://github.com/aminueza/terraform-provider-minio/issues/1205)).
- Importing a `minio_s3_bucket` now works on a backend without bucket policies,
  such as Garage. The import derives `acl` from the bucket policy and failed on
  `NotImplemented`; a backend that cannot hold a policy can only hold the
  default, so the bucket is imported as `private`. Any other policy error still
  fails the import. Importing a bucket that does not exist now says so by name,
  where it reported `Bucket name cannot be empty`
  ([#1206](https://github.com/aminueza/terraform-provider-minio/issues/1206)).
- `data.minio_s3_bucket` no longer reports `versioning_enabled = false`,
  `object_lock_enabled = false` and an empty `policy` when it fails to read
  them. It ignored the errors of those three reads, so a permission or network
  error looked like a bucket with versioning off and no policy. An error now
  fails the read; a bucket without object lock still reads as `false`. With
  `s3_compat_mode = true`, a backend that does not implement one of the three
  calls returns the zero value with a warning that names the attribute
  ([#1207](https://github.com/aminueza/terraform-provider-minio/issues/1207)).
- `minio_ilm_tier` no longer crashes the provider when `type` names a backend
  whose configuration block is missing, such as `type = "s3"` with no
  `s3_config` block. Create and update indexed the block's first element
  without checking that it existed, which crashed the whole provider process.
  They now return an error naming the block that is required
  ([#1200](https://github.com/aminueza/terraform-provider-minio/issues/1200)).

## [3.43.0] - 2026-09-15

### Added

- `minio_access_keys` data source, which lists the access keys a server holds
  grouped by identity provider, with an optional `users` filter. It reports
  identifiers and metadata only; the MinIO APIs behind it never return secret
  material. The OpenID group covers both the credential a login mints and the
  service accounts created under it, told apart by `kind`, and it asks for every
  configured OpenID provider rather than the default one alone.
  `identity_providers` defaults to `builtin`, because MinIO answers a listing
  request for an identity provider it does not run with an error rather than an
  empty list
  ([#1144](https://github.com/aminueza/terraform-provider-minio/issues/1144)).
- `minio_kms_keys` data source, which lists the keys the configured KMS holds
  and takes an optional `pattern` glob. Community MinIO serves key listing
  whenever a KMS is configured, so this is not limited to AIStor
  ([#1145](https://github.com/aminueza/terraform-provider-minio/issues/1145)).

### Changed

- Bumped `golang.org/x/sync` from 0.22.0 to 0.23.0
  ([#1165](https://github.com/aminueza/terraform-provider-minio/pull/1165)).
- Bumped the Go toolchain to go1.26.8
  ([#1167](https://github.com/aminueza/terraform-provider-minio/pull/1167)).

### Fixed

- `minio_accesskey` no longer loses its attached `policy` when the secret is
  rotated. MinIO's `UpdateServiceAccount` reads a request that omits the policy
  field as "no session policy" and detaches whatever is attached, and the
  provider sent status, secret and description changes as separate calls, none
  of which carried the policy. Rotating a secret through
  `secret_key_wo_version`, or changing only the status or the description, threw
  away a policy the configuration never asked to remove. Clearing a policy on
  purpose still works
  ([#1176](https://github.com/aminueza/terraform-provider-minio/pull/1176)).

## [3.42.0] - 2026-09-11

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

[Unreleased]: https://github.com/aminueza/terraform-provider-minio/compare/v3.46.3...HEAD
[3.46.3]: https://github.com/aminueza/terraform-provider-minio/compare/v3.46.2...v3.46.3
[3.46.2]: https://github.com/aminueza/terraform-provider-minio/compare/v3.46.1...v3.46.2
[3.46.1]: https://github.com/aminueza/terraform-provider-minio/compare/v3.46.0...v3.46.1
[3.46.0]: https://github.com/aminueza/terraform-provider-minio/compare/v3.45.1...v3.46.0
[3.45.1]: https://github.com/aminueza/terraform-provider-minio/compare/v3.45.0...v3.45.1
[3.45.0]: https://github.com/aminueza/terraform-provider-minio/compare/v3.44.0...v3.45.0
[3.44.0]: https://github.com/aminueza/terraform-provider-minio/compare/v3.43.0...v3.44.0
[3.43.0]: https://github.com/aminueza/terraform-provider-minio/compare/v3.42.0...v3.43.0
[3.42.0]: https://github.com/aminueza/terraform-provider-minio/compare/v3.41.1...v3.42.0
[3.41.1]: https://github.com/aminueza/terraform-provider-minio/compare/v3.41.0...v3.41.1
[3.41.0]: https://github.com/aminueza/terraform-provider-minio/compare/v3.40.1...v3.41.0
[3.40.1]: https://github.com/aminueza/terraform-provider-minio/compare/v3.40.0...v3.40.1
[3.40.0]: https://github.com/aminueza/terraform-provider-minio/compare/v3.39.0...v3.40.0
[3.39.0]: https://github.com/aminueza/terraform-provider-minio/compare/v3.38.6...v3.39.0
