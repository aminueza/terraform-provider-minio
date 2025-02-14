---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "minio_ilm_policy Resource - terraform-provider-minio"
subcategory: ""
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

# Simple expiration rule
resource "minio_ilm_policy" "bucket-lifecycle-rules" {
  bucket = minio_s3_bucket.bucket.bucket

  rule {
    id         = "expire-7d"
    status     = "Enabled"
    expiration = "7d"
  }
}

# Complex lifecycle policy with multiple rules
resource "minio_ilm_policy" "comprehensive-rules" {
  bucket = minio_s3_bucket.bucket.bucket

  # Rule with transition and expiration
  rule {
    id     = "documents"
    status = "Enabled"
    transition {
      days          = "30d"
      storage_class = "STANDARD_IA"
    }
    expiration = "90d"
    filter     = "documents/"
    tags = {
      "department" = "finance"
    }
  }

  # Rule with noncurrent version management
  rule {
    id     = "versioning"
    status = "Enabled"
    noncurrent_expiration {
      days           = "60d"
      newer_versions = 5
    }
    noncurrent_transition {
      days           = "30d"
      storage_class  = "GLACIER"
      newer_versions = 3
    }
  }
}
```

## Schema

### Required

- `bucket` (String) The name of the bucket to which this lifecycle policy applies. Must be between 0 and 63 characters.
- `rule` (Block List, Min: 1) A list of lifecycle rules (see below for nested schema).

### Read-Only

- `id` (String) The ID of this resource.

### Nested Schema for `rule`

#### Required

- `id` (String) Unique identifier for the rule.

#### Optional

- `status` (String) Status of the rule. Can be either "Enabled" or "Disabled". Defaults to "Enabled".
- `expiration` (String) When objects should expire. Value must be a duration (e.g., "7d"), date (e.g., "2024-12-31"), or "DeleteMarker".
- `filter` (String) Prefix path filter for the rule.
- `tags` (Map of String) Key-value pairs of tags to filter objects.
- `transition` (Block List, Max: 1) Configuration block for transitioning objects to a different storage class (see below).
- `noncurrent_transition` (Block List, Max: 1) Configuration for transitioning noncurrent object versions (see below).
- `noncurrent_expiration` (Block List, Max: 1) Configuration for expiring noncurrent object versions (see below).

#### Read-Only

- `status` (String) Current status of the rule.

### Nested Schema for `rule.transition`

#### Required

- `storage_class` (String) The storage class to transition objects to.

#### Optional

- `days` (String) Number of days after which objects should transition, in format "Nd" (e.g., "30d").
- `date` (String) Specific date for the transition in "YYYY-MM-DD" format.

### Nested Schema for `rule.noncurrent_transition`

#### Required

- `days` (String) Number of days after which noncurrent versions should transition, in format "Nd".
- `storage_class` (String) The storage class to transition noncurrent versions to.

#### Optional

- `newer_versions` (Number) Number of newer versions to retain before transition. Must be non-negative.

### Nested Schema for `rule.noncurrent_expiration`

#### Required

- `days` (String) Number of days after which noncurrent versions should be deleted, in format "Nd".

#### Optional

- `newer_versions` (Number) Number of newer versions to retain before expiration. Must be non-negative.

## Import

MinIO lifecycle policies can be imported using the bucket name:

```shell
terraform import minio_ilm_policy.example bucket-name
```
