---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "minio_s3_bucket_replication Resource - terraform-provider-minio"
subcategory: ""
description: |-
---

# minio_s3_bucket_replication (Resource)

## Example Usage

```terraform
resource "minio_s3_bucket" "my_bucket_in_a" {
  bucket = "my-bucket"
}

resource "minio_s3_bucket" "my_bucket_in_b" {
  provider = minio.deployment_b
  bucket = "my-bucket"
}

resource "minio_s3_bucket_versioning" "my_bucket_in_a" {
  bucket     = minio_s3_bucket.my_bucket_in_a.bucket

  versioning_configuration {
    status = "Enabled"
  }
}

resource "minio_s3_bucket_versioning" "my_bucket_in_b" {
  provider = minio.deployment_b
  bucket     = minio_s3_bucket.my_bucket_in_b.bucket

  versioning_configuration {
    status = "Enabled"
  }
}

data "minio_iam_policy_document" "replication_policy" {
  statement {
    sid       = "ReadBuckets"
    effect    = "Allow"
    resources = ["arn:aws:s3:::*"]

    actions = [
      "s3:ListBucket",
    ]
  }

  statement {
    sid       = "EnableReplicationOnBucket"
    effect    = "Allow"
    resources = ["arn:aws:s3:::my-bucket"]

    actions = [
      "s3:GetReplicationConfiguration",
      "s3:ListBucket",
      "s3:ListBucketMultipartUploads",
      "s3:GetBucketLocation",
      "s3:GetBucketVersioning",
      "s3:GetBucketObjectLockConfiguration",
      "s3:GetEncryptionConfiguration",
    ]
  }

  statement {
    sid       = "EnableReplicatingDataIntoBucket"
    effect    = "Allow"
    resources = ["arn:aws:s3:::my-bucket/*"]

    actions = [
      "s3:GetReplicationConfiguration",
      "s3:ReplicateTags",
      "s3:AbortMultipartUpload",
      "s3:GetObject",
      "s3:GetObjectVersion",
      "s3:GetObjectVersionTagging",
      "s3:PutObject",
      "s3:PutObjectRetention",
      "s3:PutBucketObjectLockConfiguration",
      "s3:PutObjectLegalHold",
      "s3:DeleteObject",
      "s3:ReplicateObject",
      "s3:ReplicateDelete",
    ]
  }
}

# One-Way replication (A -> B)
resource "minio_iam_policy" "replication_in_b" {
  provider = minio.deployment_b
  name   = "ReplicationToMyBucketPolicy"
  policy = data.minio_iam_policy_document.replication_policy.json
}

resource "minio_iam_user" "replication_in_b" {
  provider = minio.deployment_b
  name = "my-user"
  force_destroy = true
}

resource "minio_iam_user_policy_attachment" "replication_in_b" {
  provider = minio.deployment_b
  user_name   = minio_iam_user.replication_in_b.name
  policy_name = minio_iam_policy.replication_in_b.id
}

resource "minio_iam_service_account" "replication_in_b" {
  provider = minio.deployment_b
  target_user = minio_iam_user.replication_in_b.name

  depends_on = [
    minio_iam_user_policy_attachment.replication_in_b
  ]
}

resource "minio_s3_bucket_replication" "replication_in_b" {
  bucket     = minio_s3_bucket.my_bucket_in_a.bucket

  rule {
    delete_replication = true
    delete_marker_replication = true
    existing_object_replication = true
    metadata_sync = true # Must be true for two-way

    target {
      bucket = minio_s3_bucket.my_bucket_in_b.bucket
      secure = false
      host = var.minio_server_b
      bandwidth_limit = "100M"
      access_key = minio_iam_service_account.replication_in_b.access_key
      secret_key = minio_iam_service_account.replication_in_b.secret_key
    }
  }

  depends_on = [
    minio_s3_bucket_versioning.my_bucket_in_a,
    minio_s3_bucket_versioning.my_bucket_in_b
  ]
}

# Two-Way replication (A <-> B)
resource "minio_iam_policy" "replication_in_a" {
  name   = "ReplicationToMyBucketPolicy"
  policy = data.minio_iam_policy_document.replication_policy.json
}

resource "minio_iam_user" "replication_in_a" {
  name = "my-user"
  force_destroy = true
}

resource "minio_iam_user_policy_attachment" "replication_in_a" {
  user_name   = minio_iam_user.replication_in_a.name
  policy_name = minio_iam_policy.replication_in_a.id
}

resource "minio_iam_service_account" "replication_in_a" {
  target_user = minio_iam_user.replication_in_a.name

  depends_on = [
    minio_iam_user_policy_attachment.replication_in_b
  ]
}

resource "minio_s3_bucket_replication" "replication_in_a" {
  bucket     = minio_s3_bucket.my_bucket_in_b.bucket
  provider = minio.deployment_b

  rule {
    delete_replication = true
    delete_marker_replication = true
    existing_object_replication = true
    metadata_sync = true

    target {
      bucket = minio_s3_bucket.my_bucket_in_a.bucket
      host = var.minio_server_a
      secure = false
      bandwidth_limit = "100M"
      access_key = minio_iam_service_account.replication_in_a.access_key
      secret_key = minio_iam_service_account.replication_in_a.secret_key
    }
  }

  depends_on = [
    minio_s3_bucket_versioning.my_bucket_in_a,
    minio_s3_bucket_versioning.my_bucket_in_b,
  ]
}
```

<!-- schema generated by tfplugindocs -->

## Schema

### Required

- `bucket` (String) Name of the bucket on which to setup replication rules

### Optional

- `rule` (Block List) Rule definitions (see [below for nested schema](#nestedblock--rule))

### Read-Only

- `id` (String) The ID of this resource.

<a id="nestedblock--rule"></a>

### Nested Schema for `rule`

Required:

- `target` (Block List, Min: 1, Max: 1) Bucket prefix (see [below for nested schema](#nestedblock--rule--target))

Optional:

- `delete_marker_replication` (Boolean) Whether or not to synchronise marker deletion
- `delete_replication` (Boolean) Whether or not to propagate deletion
- `enabled` (Boolean) Whether or not this rule is enabled
- `existing_object_replication` (Boolean) Whether or not to synchronise object created prior the replication configuration
- `metadata_sync` (Boolean) Whether or not to synchonise buckets and objects metadata (such as locks). This must be enabled to achieve a two-way replication
- `prefix` (String) Bucket prefix object must be in to be syncronised
- `priority` (Number) Rule priority. If omitted, the inverted index will be used as priority. This means that the first rule definition will have the higher priority
- `tags` (Map of String) Tags which objects must have to be syncronised

Read-Only:

- `arn` (String) Rule ARN genrated by MinIO
- `id` (String) Rule ID generated by MinIO

<a id="nestedblock--rule--target"></a>

### Nested Schema for `rule.target`

Required:

- `access_key` (String) Access key for the replication service account in the target MinIO
- `bucket` (String) The name of the existing target bucket to replicate into
- `host` (String) The target host (pair IP/port or domain port). If port is omitted, HTTPS port (or HTTP if unsecure) will be used. This host must be reachable by the MinIO instance itself

Optional:

- `bandwidth_limit` (String) Maximum bandwidth in byte per second that MinIO can used when syncronysing this target. Minimum is 100MB
- `disable_proxy` (Boolean) Disable proxy for this target
- `health_check_period` (String) Period where the health of this target will be checked. This must be a valid duration, such as `5s` or `2m`
- `path` (String) Path of the Minio endpoint. This is usefull if MinIO API isn't served on at the root, e.g for `example.com/minio/`, the path would be `/minio/`
- `path_style` (String) Whether to use path-style or virtual-hosted-syle request to this target (https://docs.aws.amazon.com/AmazonS3/latest/userguide/VirtualHosting.html#path-style-access). `auto` allows MinIO to chose automatically the appropriate option (Recommened)`
- `region` (String) Region of the target MinIO. This will be used to generate the target ARN
- `secret_key` (String, Sensitive) Secret key for the replication service account in the target MinIO. This is optional so it can be imported but prevent secret update
- `secure` (Boolean) Whether to use HTTPS with this target (Recommended). Note that disabling HTTPS will yield Terraform warning for security reason`
- `storage_class` (String) The storage class to use for the object on this target
- `syncronous` (Boolean) Use synchronous replication.
