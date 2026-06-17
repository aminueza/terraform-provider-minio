# MinIO provider — resources reference

Exact schemas and the full inventory for `aminueza/minio`. Argument names are
verified against the provider's generated docs. When in doubt about a resource
not detailed here, run `terraform providers schema -json` or check
`https://registry.terraform.io/providers/aminueza/minio/latest/docs/resources/<name>`
(URL omits the `minio_` prefix).

## Contents

- [Full inventory](#full-inventory) — every resource, grouped, one line each
- [Exact schemas: S3 bucket & object](#exact-schemas-s3-bucket--object)
- [Exact schemas: IAM](#exact-schemas-iam)
- [Clarifications](#clarifications) — which of several similar resources to use
- [Import syntax](#import-syntax)

---

## Full inventory

### S3 bucket & object
- `minio_s3_bucket` — create/manage a bucket (name, acl, force_destroy, object lock).
- `minio_s3_bucket_policy` — full bucket policy from raw JSON.
- `minio_s3_bucket_anonymous_access` — canned/anonymous public access for a bucket.
- `minio_s3_bucket_versioning` — versioning configuration.
- `minio_s3_bucket_replication` — bucket-to-bucket replication rules.
- `minio_s3_bucket_retention` — default object-lock retention mode/period.
- `minio_s3_bucket_object_lock_configuration` — object-lock configuration.
- `minio_s3_bucket_notification` — event notifications (queue/topic/lambda targets).
- `minio_s3_bucket_server_side_encryption` — bucket default SSE (AES256 / aws:kms).
- `minio_s3_bucket_cors` — CORS rules.
- `minio_s3_bucket_quota` — bucket size quota.
- `minio_s3_bucket_lifecycle` — lifecycle (ILM) rules, AWS-parity block schema. **Preferred.**
- `minio_s3_bucket_tags` — bucket tags.
- `minio_s3_object` — upload/manage an object.
- `minio_s3_object_tags` — object tags.
- `minio_s3_object_legal_hold` — object legal hold.
- `minio_s3_object_retention` — per-object retention.
- `minio_s3_incomplete_upload_cleanup` — abort/clean incomplete multipart uploads.

### IAM
- `minio_iam_user` — IAM user.
- `minio_iam_group` — IAM group.
- `minio_iam_group_membership` — group-centric authoritative membership set.
- `minio_iam_user_group_membership` — user-centric authoritative group set.
- `minio_iam_group_user_attachment` — single user↔group edge (non-authoritative).
- `minio_iam_service_account` — service account; emits access/secret key pair.
- `minio_iam_policy` — managed (named) policy.
- `minio_iam_group_policy` — inline policy embedded on a group.
- `minio_iam_user_policy_attachment` — attach a managed policy to a user.
- `minio_iam_group_policy_attachment` — attach a managed policy to a group.
- `minio_iam_import` — bulk import of IAM entities.

### LDAP / external IdP
- `minio_iam_ldap_user_policy_attachment` — attach policy to an LDAP user DN.
- `minio_iam_ldap_group_policy_attachment` — attach policy to an LDAP group DN.
- `minio_iam_idp_openid` — OpenID Connect IdP configuration.
- `minio_iam_idp_ldap` — LDAP IdP configuration.

### ILM / KMS / access keys
- `minio_ilm_policy` — **legacy** lifecycle (string durations); see clarification vs `minio_s3_bucket_lifecycle`.
- `minio_ilm_tier` — remote transition tier (e.g. to another S3/cloud).
- `minio_kms_key` — KMS key.
- `minio_accesskey` — access key for a user (write-only secret; not exported).

### Server configuration
- `minio_config` — generic server config key/value.
- `minio_config_restore` — restore server config from a backup.
- `minio_server_config_api` — typed server config: API subsystem.
- `minio_server_config_region` — typed server config: region/bucket lookup.
- `minio_server_config_scanner` — typed server config: object scanner.
- `minio_server_config_heal` — typed server config: auto-heal.
- `minio_server_config_storage_class` — typed server config: storage classes.
- `minio_server_config_etcd` — typed server config: etcd.
- `minio_audit_webhook` / `minio_audit_kafka` — audit log targets.
- `minio_logger_webhook` — logger webhook target.
- `minio_site_replication` — multi-site (active-active) replication setup.
- `minio_prometheus_bearer_token` — Prometheus scrape bearer token.

### Notification targets (server-side event destinations)
- `minio_notify_webhook`, `minio_notify_amqp`, `minio_notify_kafka`,
  `minio_notify_mqtt`, `minio_notify_nats`, `minio_notify_nsq`,
  `minio_notify_mysql`, `minio_notify_postgres`,
  `minio_notify_elasticsearch`, `minio_notify_redis`.

### Pool / batch / service
- `minio_pool_rebalance` — trigger a pool rebalance.
- `minio_pool_decommission` — decommission a pool.
- `minio_batch_job` — server-side batch job (replicate/expire/keyrotate).
- `minio_bucket_metadata_import` — import bucket metadata.
- `minio_service_action` — restart/stop service action.

---

## Exact schemas: S3 bucket & object

### `minio_s3_bucket`
All args optional. Use **either** `bucket` or `bucket_prefix` (prefix is create-only).
```hcl
resource "minio_s3_bucket" "this" {
  bucket         = "my-bucket"      # OR bucket_prefix; exact name
  acl            = "private"        # default "private"
  force_destroy  = false            # true deletes all objects (incl. locked) on destroy
  object_locking = false            # enable object lock at creation (immutable later)
  quota          = 1073741824       # bytes (optional)
  tags           = { Env = "prod" }
}
```
- `acl` canned values: `private` (default), `public`, `public-read`, `public-read-write`, `public-write`.
- Exported: `arn`, `bucket_domain_name`, `id`.

### `minio_s3_bucket_policy`
```hcl
resource "minio_s3_bucket_policy" "this" {
  bucket = "my-bucket"            # REQUIRED
  policy = jsonencode({ ... })    # REQUIRED — full policy JSON
}
```

### `minio_s3_bucket_anonymous_access`
```hcl
resource "minio_s3_bucket_anonymous_access" "this" {
  bucket      = "my-bucket"       # REQUIRED
  access_type = "public-read"     # public | public-read | public-read-write | public-write
  # policy    = jsonencode({...}) # optional custom JSON; wins over access_type if both set
}
```
Do **not** also put a `minio_s3_bucket_policy` on the same bucket — they write the same underlying policy.

### `minio_s3_bucket_versioning`
```hcl
resource "minio_s3_bucket_versioning" "this" {
  bucket = "my-bucket"            # REQUIRED
  versioning_configuration {      # REQUIRED (max 1)
    status            = "Enabled" # Enabled | Suspended
    exclude_folders   = false
    excluded_prefixes = ["tmp/"]
  }
}
```

### `minio_s3_bucket_lifecycle` (preferred lifecycle resource)
Integer `days`; block-form `filter`/`expiration`/`transition`.
```hcl
resource "minio_s3_bucket_lifecycle" "this" {
  bucket = "my-bucket"            # REQUIRED
  rule {                          # REQUIRED (≥1)
    id     = "expire-logs"        # REQUIRED (≤255 chars)
    status = "Enabled"            # Enabled | Disabled (default Enabled)

    filter {                                   # optional (max 1)
      prefix                   = "logs/"       # cannot combine with top-level `tag`
      object_size_greater_than = 10485760
      object_size_less_than    = 104857600
      tag { key = "k" value = "v" }            # single tag (max 1)
      and {                                    # composite AND (max 1)
        prefix                   = "reports/"
        tags                     = { retain = "long" }
        object_size_greater_than = 1024
      }
    }
    expiration {                               # optional (max 1)
      days                         = 30        # int; mutually exclusive with `date`
      # date                       = "2025-12-31"
      expired_object_delete_marker = false
    }
    transition {                               # optional (max 1)
      storage_class = "GLACIER"                # REQUIRED in block
      days          = 60                       # int; OR `date`
    }
    noncurrent_version_expiration {            # needs versioning
      noncurrent_days           = 180          # REQUIRED int
      newer_noncurrent_versions = 3
    }
    noncurrent_version_transition {            # needs versioning
      noncurrent_days = 90                      # REQUIRED int
      storage_class   = "GLACIER"               # REQUIRED
    }
    abort_incomplete_multipart_upload {
      days_after_initiation = 7                 # REQUIRED int; can't combine with tag filters
    }
  }
}
```

### `minio_ilm_policy` (legacy)
String durations (`"90d"`); scalar `filter`/`expiration`. Same underlying config as
`minio_s3_bucket_lifecycle` — never use both on one bucket.
```hcl
resource "minio_ilm_policy" "this" {
  bucket = "my-bucket"            # REQUIRED
  rule {                          # REQUIRED (≥1)
    id         = "rule1"          # REQUIRED
    status     = "Enabled"
    expiration = "90d"            # STRING: "5d" | "1970-01-01" | "DeleteMarker"
    filter     = "documents/"     # STRING prefix
    tags       = { env = "test" }
    transition {
      storage_class = "GLACIER"   # REQUIRED
      days          = "30d"       # STRING "Nd"; OR date "YYYY-MM-DD"
    }
    noncurrent_expiration {
      days           = "365d"     # REQUIRED STRING
      newer_versions = 5
    }
  }
}
```

### `minio_s3_bucket_replication`
Source bucket must be versioned. `target.secret_key` is sensitive.
```hcl
resource "minio_s3_bucket_replication" "this" {
  bucket         = "source-bucket"  # REQUIRED (versioned)
  resync_version = 0                # bump to resync existing objects
  rule {
    enabled                     = true
    delete_replication          = true
    delete_marker_replication   = true
    existing_object_replication = true
    metadata_sync               = true   # must be true for two-way
    prefix                      = "data/"
    priority                    = 1
    target {                            # REQUIRED (max 1)
      bucket          = "target-bucket" # REQUIRED
      host            = "minio-b:9000"  # REQUIRED reachable host
      access_key      = "..."           # REQUIRED
      secret_key      = "..."           # sensitive (optional so it can be imported)
      secure          = true
      bandwidth_limit = "100M"          # min 100MB
      region          = "us-east-1"
      path_style      = "auto"
      synchronous     = false           # NOTE: misspelled `syncronous` alias is deprecated
    }
  }
}
```
Avoid deprecated aliases `bandwidth_limt`, `syncronous`. Exported: `last_resync_id`, per-rule `rule.arn`/`rule.id`.

### `minio_s3_bucket_server_side_encryption`
```hcl
resource "minio_s3_bucket_server_side_encryption" "this" {
  bucket          = "my-bucket"   # REQUIRED
  encryption_type = "aws:kms"     # REQUIRED: "AES256" (SSE-S3) | "aws:kms" (SSE-KMS)
  kms_key_id      = "my-kms-key"  # REQUIRED when encryption_type = "aws:kms"
}
```

### `minio_s3_object`
Note the names: `bucket_name` / `object_name` (not `bucket`/`key`).
```hcl
resource "minio_s3_object" "this" {
  bucket_name  = "my-bucket"      # REQUIRED
  object_name  = "path/text.txt"  # REQUIRED
  content      = "hello"          # use ONE of content / content_base64 / source
  # content_base64 = "..."
  # source         = "./file.txt"
  content_type = "text/plain"
  acl          = "private"        # private | public-read | public-read-write | authenticated-read
  # also: cache_control, content_encoding, content_disposition, expires, storage_class, metadata
}
```

---

## Exact schemas: IAM

### `minio_iam_user`
```hcl
resource "minio_iam_user" "this" {
  name          = "test-user"     # REQUIRED
  secret        = var.user_secret # sensitive; stored in state
  update_secret = false           # set true to rotate `secret`
  disable_user  = false
  force_destroy = false           # delete even if it has non-TF-managed access keys
  tags          = { team = "data" }
  # secret_wo         = var.user_secret  # write-only (NOT stored in state); Terraform ≥ 1.11
  # secret_wo_version = 1                # bump to rotate when using secret_wo
}
```
Exported: `status`, `id`.

### `minio_iam_group`
```hcl
resource "minio_iam_group" "this" {
  name          = "developers"    # REQUIRED
  disable_group = false
  force_destroy = false
}
```
Exported: `group_name`.

### `minio_iam_group_membership` (group-centric, authoritative)
```hcl
resource "minio_iam_group_membership" "this" {
  name  = "dev-membership"        # REQUIRED — name of THIS resource
  group = "developers"            # REQUIRED — target group
  users = ["alice", "bob"]        # REQUIRED — Set of String
}
```

### `minio_iam_user_group_membership` (user-centric, authoritative)
```hcl
resource "minio_iam_user_group_membership" "this" {
  user   = "alice"                     # REQUIRED
  groups = ["developers", "admins"]    # REQUIRED — user ends up in EXACTLY these
}
```

### `minio_iam_group_user_attachment` (single edge, non-authoritative)
```hcl
resource "minio_iam_group_user_attachment" "this" {
  group_name = "developers"       # REQUIRED
  user_name  = "alice"            # REQUIRED
}
```

### `minio_iam_policy` (managed)
```hcl
resource "minio_iam_policy" "this" {
  policy      = data.minio_iam_policy_document.this.json  # REQUIRED (JSON string)
  name        = "my-policy"        # conflicts with name_prefix
  # name_prefix = "my-policy-"
}
```

### `minio_iam_group_policy` (inline on a group)
```hcl
resource "minio_iam_group_policy" "this" {
  group  = "developers"            # REQUIRED
  policy = jsonencode({ ... })     # REQUIRED
  name   = "inline-rule"           # random if omitted; conflicts with name_prefix
}
```

### `minio_iam_user_policy_attachment`
Names: `user_name` / `policy_name`.
```hcl
resource "minio_iam_user_policy_attachment" "this" {
  user_name   = minio_iam_user.this.id
  policy_name = minio_iam_policy.this.id
}
```

### `minio_iam_group_policy_attachment`
Names: `group_name` / `policy_name`.
```hcl
resource "minio_iam_group_policy_attachment" "this" {
  group_name  = minio_iam_group.this.id
  policy_name = minio_iam_policy.this.id
}
```

### `minio_iam_service_account`
Mints credentials; **exports** the secret (sensitive) so other resources can use it.
```hcl
resource "minio_iam_service_account" "this" {
  target_user  = minio_iam_user.this.name   # REQUIRED — owning user
  name         = "svc"                       # optional, ≤32 bytes, can't be cleared once set
  description  = "CI uploader"               # optional, ≤256 bytes
  policy       = data.minio_iam_policy_document.scoped.json  # optional — restricts the SA
  disable_user = false                       # disable the SA (there is NO `status` input)
  expiration   = "2026-12-31T00:00:00Z"      # optional RFC3339 (now+15min .. now+365d)
  update_secret = false                      # rotate the secret key
  # secret_key_wo         = "..."            # write-only; Terraform ≥ 1.11
  # secret_key_wo_version = 1
}
# Exported (SENSITIVE): access_key, secret_key; plus status.
```

### `minio_accesskey`
Like a service account but the secret is **write-only and never exported**.
```hcl
resource "minio_accesskey" "this" {
  user               = minio_iam_user.this.name  # REQUIRED
  access_key         = "MYAPPKEY12345"            # optional, 8-20 chars; auto-generated if omitted
  secret_key         = var.app_secret             # optional, ≥8 chars; WRITE-ONLY, not in state
  secret_key_version = "v1"                        # required for change detection when secret_key set
  status             = "enabled"                   # enabled | disabled
  policy             = jsonencode({ ... })         # optional — policy NAME or JSON
}
# secret_key is NOT exported. Supply/track it yourself (e.g. from a secret store).
```

---

## Clarifications

**Group membership trio**
- `minio_iam_group_membership` (`name` + `group` + `users`) — group-centric,
  authoritative over that named set. *Use when* you own a group's full member list.
- `minio_iam_user_group_membership` (`user` + `groups`) — user-centric,
  authoritative; the user ends up in exactly those groups. *Use when* you own one
  user's complete group set.
- `minio_iam_group_user_attachment` (`group_name` + `user_name`) — one edge,
  non-authoritative. *Use when* adding one user to one group (composes with `for_each`).

**`minio_s3_bucket_lifecycle` vs `minio_ilm_policy`** — same underlying config,
never both on one bucket. Lifecycle uses AWS-parity blocks with integer `days`;
ilm_policy uses scalar strings and durations like `"90d"`. Prefer
`minio_s3_bucket_lifecycle` for new work; `minio_ilm_policy` only for legacy.

**`minio_s3_bucket_anonymous_access` vs `minio_s3_bucket_policy`** — both write
the bucket's single policy; never both. Anonymous-access gives canned
`access_type`; bucket_policy takes full custom JSON. Use anonymous-access for
simple public exposure, bucket_policy for cross-principal/custom rules.

**`minio_iam_service_account` vs `minio_accesskey`** — both mint creds for a user.
Service account exports `access_key` + `secret_key` (use them downstream, e.g. a
replication target); accesskey's `secret_key` is write-only and never exported
(supply it from a secret store, never lands in state). Choose by whether
Terraform needs the secret value downstream.

**`secret` / `update_secret` vs `secret_wo`** — `secret` stores the key in state;
`update_secret=true` rotates it. `secret_wo` + `secret_wo_version` is write-only
(never persisted); bump the version to rotate. Write-only args need Terraform ≥ 1.11.

**Inline vs managed policy** — inline (`minio_iam_group_policy`) embeds JSON that
lives and dies with the group. Managed (`minio_iam_policy`) is reusable and is
attached via `minio_iam_{user,group}_policy_attachment`. Use inline for one-off
group rules, managed+attachment for policies shared across principals.

---

## Import syntax

Every resource is importable. IDs by resource:

| Resource | Import ID | Example |
|---|---|---|
| `minio_s3_bucket` | bucket name | `terraform import minio_s3_bucket.b my-bucket` |
| `minio_s3_bucket_policy` | bucket name | `... minio_s3_bucket_policy.p my-bucket` |
| `minio_s3_bucket_versioning` | bucket name | `... .v my-bucket` |
| `minio_s3_bucket_lifecycle` | bucket name | `... .l my-bucket` (re-apply once to reconcile) |
| `minio_s3_bucket_replication` | bucket name | `... .r my-bucket` |
| `minio_s3_object` | `bucket/object` | `... .o my-bucket/path/file.txt` |
| `minio_iam_user` | user name | `... minio_iam_user.u alice` |
| `minio_iam_group` | group name | `... minio_iam_group.g developers` |
| `minio_iam_group_membership` | group name | `... .m developers` |
| `minio_iam_user_group_membership` | user name | `... .m alice` |
| `minio_iam_group_user_attachment` | `group/user` | `... .a developers/alice` |
| `minio_iam_policy` | policy name | `... minio_iam_policy.p my-policy` |
| `minio_iam_user_policy_attachment` | `user/policy` | `... .a alice/my-policy` |
| `minio_iam_group_policy_attachment` | `group/policy` | `... .a developers/my-policy` |
| `minio_iam_service_account` | access key id | `... .sa AKIA...` |
| `minio_accesskey` | access key id | `... .k MYAPPKEY12345` |

Prefer the `import {}` config block (Terraform ≥ 1.5) so the import is reviewed
in a plan — see `recipes.md` → *Import existing resources*.
