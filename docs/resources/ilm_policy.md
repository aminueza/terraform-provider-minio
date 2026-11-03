---
page_title: "minio_ilm_policy Resource - terraform-provider-minio"
subcategory: "ILM"
description: |-
  minio_ilm_policy handles lifecycle settings for a given minio_s3_bucket.
---

# minio_ilm_policy (Resource)

`minio_ilm_policy` handles lifecycle settings for a given `minio_s3_bucket`.

## Example Usage

```terraform
resource "minio_s3_bucket" "bucket" {
  bucket = "bucket"
}

resource "minio_ilm_policy" "test_policy" {
  bucket = minio_s3_bucket.bucket.bucket

  rule {
    id     = "rule1"
    status = "Enabled"
    expiration = "90d"
    filter     = "documents/"
    tags = {
      environment = "test"
      type        = "document"
    }
  }

  rule {
    id     = "rule2"
    status = "Disabled"
    transition {
      days          = "30d"
      storage_class = "GLACIER"
    }
    filter = "backups/"
  }

  rule {
    id = "rule3"
    transition {
      date          = "2025-12-31"  # Example: transition on a specific date
      storage_class = "STANDARD_IA"
    }
    filter = "archives/"
  }

  rule {
    id = "rule4"
    noncurrent_transition {
      days           = "45d"
      storage_class  = "GLACIER"
      newer_versions = 3
    }
    filter = "versioned-data/"
  }

  rule {
    id = "rule5"
    noncurrent_expiration {
      days           = "365d"
      newer_versions = 5
    }
    filter = "old-versions/"
  }

  rule {
    id = "rule6"
    transition {
      days          = "60d"
      storage_class = "STANDARD_IA"
    }
    expiration = "180d"
    filter     = "logs/"
    tags = {
      retention = "short-term"
      type      = "logs"
    }
  }

  rule {
    id = "rule7"
    expired_object_delete_marker = true
    noncurrent_expiration {
      days = "30d"
    }
    filter = "versioned/"
  }

  rule {
    id = "rule8"
    # MinIO AIStor only: purge objects whose latest version is a delete marker
    all_versions_expiration {
      days          = "1d"
      delete_marker = true
    }
    filter = "purged/"
  }
}
```

## Important: One Resource Per Bucket

Each bucket must have exactly **one** `minio_ilm_policy` resource. All lifecycle rules for a bucket must be defined within a single resource. Creating multiple `minio_ilm_policy` resources for the same bucket will cause them to overwrite each other, leading to unexpected behavior.

**Incorrect** (rules will overwrite each other):

```terraform
resource "minio_ilm_policy" "daily_cleanup" {
  bucket = "my-bucket"
  rule {
    id         = "daily_cleanup"
    expiration = "14d"
    filter     = "daily/"
  }
}

resource "minio_ilm_policy" "weekly_cleanup" {
  bucket = "my-bucket"  # Same bucket - this will overwrite the above!
  rule {
    id         = "weekly_cleanup"
    expiration = "80d"
    filter     = "weekly/"
  }
}
```

**Correct** (all rules in one resource):

```terraform
resource "minio_ilm_policy" "my_bucket_lifecycle" {
  bucket = "my-bucket"

  rule {
    id         = "daily_cleanup"
    expiration = "14d"
    filter     = "daily/"
  }

  rule {
    id         = "weekly_cleanup"
    expiration = "80d"
    filter     = "weekly/"
  }
}
```

## Limitations

Abort-only lifecycle rules are not persisted by the MinIO server. If you configure a rule with only `abort_incomplete_multipart_upload`, the provider preserves that rule in Terraform state to avoid perpetual diffs, but it is not sent to the server. Combine it with at least one other action (e.g., `expiration` or `transition`) to persist on the server.

`all_versions_expiration` maps the `AllVersionsExpiration` lifecycle action, which only MinIO AIStor supports. MinIO community servers reject a lifecycle configuration that contains it. It matches the `mc ilm rule add` flags as follows:

| `mc ilm rule add` flag | Attribute |
|------------------------|-----------|
| `--purge-all-object-versions-days N` | `days = "Nd"` |
| `--purge-all-object-versions-delete-marker` | `delete_marker = true` |

- With `delete_marker = true`, all versions of objects whose latest version is a delete marker are purged, the delete marker included, `days` after the delete marker was created. Objects whose latest version is not a delete marker are left alone. This is the setting that cleans up delete markers left behind by a time-based `expiration` on a versioned bucket.
- Without `delete_marker`, all versions of every object matching the rule, live objects included, are expired `days` after the latest version was created, and delete markers are not removed. `days = "1d"` in this mode empties the matching objects after one day.

## Mutually Exclusive Attributes

The following attributes within a `rule` block are **mutually exclusive** per MinIO/S3 lifecycle behavior:

| Attribute | Conflicts With |
|-----------|----------------|
| `expiration` (days/date) | `expired_object_delete_marker` |
| `expired_object_delete_marker` | `expiration` (days/date) |
| `expired_object_delete_marker` | `tags` |
| `expiration = "DeleteMarker"` | `tags` |

**Note:** `expiration = "DeleteMarker"` is equivalent to `expired_object_delete_marker = true`.

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `bucket` (String) Name of the bucket.
- `rule` (Block List, Min: 1) List of lifecycle rules. (see [below for nested schema](#nestedblock--rule))

### Read-Only

- `id` (String) The ID of this resource.

<a id="nestedblock--rule"></a>
### Nested Schema for `rule`

Required:

- `id` (String) Unique identifier for the rule.

Optional:

- `abort_incomplete_multipart_upload` (Block List, Max: 1) Configuration for aborting incomplete multipart uploads. (see [below for nested schema](#nestedblock--rule--abort_incomplete_multipart_upload))
- `all_versions_expiration` (Block List, Max: 1) Expiration of all versions of an object: the `AllVersionsExpiration` action, MinIO AIStor only. One block sets one action; `days` and `delete_marker` fill its two fields, like the `mc ilm rule add` flags `--purge-all-object-versions-days` and `--purge-all-object-versions-delete-marker` do. (see [below for nested schema](#nestedblock--rule--all_versions_expiration))
- `expiration` (String) Value may be duration (5d) or date (1970-01-01) to expire objects
- `expired_object_delete_marker` (Boolean) Whether to delete the delete marker when the object has a single version (i.e., all other versions have been expired by `noncurrent_version_expiration_days`).
- `filter` (String) Object key prefix to filter which objects the rule applies to.
- `noncurrent_expiration` (Block List, Max: 1) Expiration configuration for noncurrent object versions. (see [below for nested schema](#nestedblock--rule--noncurrent_expiration))
- `noncurrent_transition` (Block List, Max: 1) Transition configuration for noncurrent object versions. (see [below for nested schema](#nestedblock--rule--noncurrent_transition))
- `status` (String) Status of the rule. Can be either 'Enabled' or 'Disabled'. Defaults to 'Enabled'.
- `tags` (Map of String) Key-value map of object tags to filter which objects the rule applies to.
- `transition` (Block List, Max: 1) Transition configuration for current object versions. (see [below for nested schema](#nestedblock--rule--transition))

<a id="nestedblock--rule--abort_incomplete_multipart_upload"></a>
### Nested Schema for `rule.abort_incomplete_multipart_upload`

Required:

- `days_after_initiation` (String) Number of days after which incomplete multipart uploads should be aborted, in format 'Nd'.


<a id="nestedblock--rule--all_versions_expiration"></a>
### Nested Schema for `rule.all_versions_expiration`

Required:

- `days` (String) Number of days, in format 'Nd' (at least 1d), as set by `--purge-all-object-versions-days`. Without `delete_marker`, all versions of an object, live objects included, are expired `days` after its latest version was created, and delete markers are not removed. With `delete_marker = true`, it is the delay after the delete marker was created.

Optional:

- `delete_marker` (Boolean) If true, purges all versions of objects whose latest version is a delete marker, including the delete marker, as set by `--purge-all-object-versions-delete-marker`.


<a id="nestedblock--rule--noncurrent_expiration"></a>
### Nested Schema for `rule.noncurrent_expiration`

Required:

- `days` (String) Number of days after becoming noncurrent to expire, in format 'Nd'.

Optional:

- `newer_versions` (Number) Number of newer versions to retain.


<a id="nestedblock--rule--noncurrent_transition"></a>
### Nested Schema for `rule.noncurrent_transition`

Required:

- `days` (String) Number of days after becoming noncurrent to transition, in format 'Nd'.
- `storage_class` (String) Target storage class for noncurrent version transition.

Optional:

- `newer_versions` (Number) Number of newer versions to retain.


<a id="nestedblock--rule--transition"></a>
### Nested Schema for `rule.transition`

Required:

- `storage_class` (String) Target storage class for the transition.

Optional:

- `date` (String) Date after which objects are transitioned, in format 'YYYY-MM-DD'.
- `days` (String) Number of days after object creation to transition, in format 'Nd'.

## Import

Import is supported using the following syntax:

```shell
# ILM policies can be imported using the bucket name
terraform import minio_ilm_policy.example bucket-name
```
