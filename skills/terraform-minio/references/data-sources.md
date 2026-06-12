# MinIO provider — data sources reference

Data sources read existing MinIO state into your config (or build policy JSON).
Argument names verified against `aminueza/minio` generated docs. For a source not
detailed here, run `terraform providers schema -json` or see
`https://registry.terraform.io/providers/aminueza/minio/latest/docs/data-sources/<name>`.

## Full inventory

### IAM
- `minio_iam_policy_document` — build AWS-IAM-style policy JSON (`.json` output).
- `minio_iam_policy` — read a managed policy.
- `minio_iam_user` / `minio_iam_users` — read one user / list users.
- `minio_iam_group` / `minio_iam_groups` — read one group / list groups.
- `minio_iam_user_policies` — policies attached to a user.
- `minio_iam_service_accounts` — list service accounts.
- `minio_iam_export` — export IAM entities.

### S3 bucket & object
- `minio_s3_bucket` / `minio_s3_buckets` — read one bucket / list buckets.
- `minio_s3_object` / `minio_s3_objects` — read one object / list objects.
- `minio_s3_bucket_policy` — bucket policy JSON.
- `minio_s3_bucket_tags` — bucket tags.
- `minio_s3_bucket_versioning` — versioning state.
- `minio_s3_bucket_encryption` — SSE config.
- `minio_s3_bucket_notification_config` — notification config.
- `minio_s3_bucket_cors_config` — CORS config.
- `minio_s3_bucket_retention` — default retention.
- `minio_s3_bucket_quota` — quota.
- `minio_s3_bucket_object_lock_configuration` — object-lock config.
- `minio_s3_bucket_replication` — replication rules.
- `minio_s3_bucket_replication_status` / `minio_s3_bucket_replication_metrics` — replication health/metrics.
- `minio_s3_bucket_anonymous_access` — current anonymous access.

### ILM / KMS
- `minio_ilm_policy` — lifecycle config.
- `minio_ilm_tiers` / `minio_ilm_tier_stats` — remote tiers and their stats.
- `minio_kms_status` / `minio_kms_metrics` — KMS health/metrics.

### Server / cluster info (no required args — great for inspection)
- `minio_server_info` — version, edition, deployment id, per-server drives.
- `minio_account_info` — account/usage info.
- `minio_storage_info` — total/used/available space, disk counts/states.
- `minio_data_usage` — data usage stats.
- `minio_health_status` — healthy/live/ready/quorum booleans.
- `minio_config_history` — config change history.
- `minio_license_info` — license info.
- `minio_prometheus_scrape_config` — Prometheus scrape config.
- `minio_pool_status` / `minio_pool_rebalance_status` — pool state / rebalance progress.
- `minio_bucket_metadata_export` — export bucket metadata.

### Batch
- `minio_batch_jobs` — list batch jobs.
- `minio_batch_job_template` — generate a batch job template.

### Notification targets
- `minio_notify_webhook`, `minio_notify_amqp`, `minio_notify_kafka`,
  `minio_notify_mqtt`, `minio_notify_nats`, `minio_notify_nsq`,
  `minio_notify_mysql`, `minio_notify_postgres`,
  `minio_notify_elasticsearch`, `minio_notify_redis`.

---

## Exact schemas (common)

### `minio_iam_policy_document` — the policy JSON builder
Use this instead of hand-writing policy JSON, then feed `.json` into a
`minio_iam_policy` / `minio_iam_group_policy` / service-account `policy`.
```hcl
data "minio_iam_policy_document" "this" {
  version       = "2012-10-17"   # optional
  policy_id     = "..."          # optional
  source_json   = "..."          # optional base doc to extend  (NOTE: singular, not source_policy)
  override_json = "..."          # optional overrides           (NOTE: singular)

  statement {                    # repeatable
    sid           = "ReadBucket"
    effect        = "Allow"      # Allow | Deny
    actions       = ["s3:GetObject", "s3:ListBucket"]
    resources     = ["arn:aws:s3:::my-bucket", "arn:aws:s3:::my-bucket/*"]
    not_resources = []           # optional
    principal     = "*"          # optional (singular String)
    not_principal = ""           # optional
    condition {                  # optional, repeatable (a Set)
      test     = "StringLike"    # REQUIRED
      variable = "s3:prefix"     # REQUIRED
      values   = ["home/"]       # REQUIRED
    }
  }
}
# Exported: json
```
Gotchas: it's `resources`/`not_resources` (plural lists) and `principal`/`not_principal`
(singular strings); merge args are `source_json` / `override_json` (singular), **not**
`source_policy`/`override_policy_documents`.

### `minio_s3_bucket` / `minio_s3_buckets`
```hcl
data "minio_s3_bucket" "one" {
  bucket = "my-bucket"           # REQUIRED
}
# exports: object_lock_enabled (bool), policy (string), region (string), versioning_enabled (bool)

data "minio_s3_buckets" "all" {
  name_prefix = "app-"           # optional
}
# exports: buckets = [{ name, creation_date }]
```

### `minio_iam_user` / `minio_iam_users`
```hcl
data "minio_iam_user" "one" {
  name = "alice"                 # REQUIRED
}
# exports: status, member_of_groups ([]string), policy_names ([]string), tags (map)

data "minio_iam_users" "list" {
  name_prefix = "svc-"           # optional
  status      = "enabled"        # optional: enabled | disabled | all
}
# exports: users = [{ name, status, member_of_groups, policy_names }]
```

### Inspection sources (no args)
```hcl
data "minio_server_info"   "srv" {}  # version, commit, deployment_id, region, edition, servers[].drives[...]
data "minio_health_status" "h"   {}  # healthy, live, ready, read_quorum, write_quorum, safe_for_maintenance
data "minio_storage_info"  "s"   {}  # total_space, used_space, available_space, disk_count, online/offline_disks, disks[...]
```
Use these to confirm the cluster is healthy and to read region/edition before
authoring config that depends on them.
