# MinIO provider — recipes

Copy-pasteable, end-to-end configs using verified argument names. Adjust names,
regions, and ARNs to the user's reality — **never apply with these placeholders**.
Always `terraform plan` and confirm before `apply`.

## Contents
- [0. Provider bootstrap](#0-provider-bootstrap)
- [1. Private bucket + versioning + lifecycle](#1-private-bucket--versioning--lifecycle)
- [2. Public (anonymous-read) bucket](#2-public-anonymous-read-bucket)
- [3. Read-only user for a bucket](#3-read-only-user-for-a-bucket)
- [4. App credentials via service account](#4-app-credentials-via-service-account)
- [5. Group + managed policy + membership](#5-group--managed-policy--membership)
- [6. Bucket replication](#6-bucket-replication)
- [7. Server-side encryption](#7-server-side-encryption)
- [8. Import existing resources](#8-import-existing-resources)

---

## 0. Provider bootstrap

```hcl
# versions.tf
terraform {
  required_version = ">= 1.5"
  required_providers {
    minio = {
      source  = "aminueza/minio"
      version = ">= 3.0.0"
    }
  }
}

# variables.tf
variable "minio_server" {
  type = string
}
variable "minio_user" {
  type      = string
  sensitive = true
}
variable "minio_password" {
  type      = string
  sensitive = true
}

# provider.tf
provider "minio" {
  minio_server   = var.minio_server   # host:port, no scheme
  minio_region   = "us-east-1"
  minio_user     = var.minio_user
  minio_password = var.minio_password
  minio_ssl      = true
}
```
Feed credentials from the environment, never commit them:
```bash
export TF_VAR_minio_server="minio.example.com:9000"
export TF_VAR_minio_user="$MINIO_ROOT_USER"
export TF_VAR_minio_password="$MINIO_ROOT_PASSWORD"
# (or skip the vars entirely and let the provider read MINIO_ENDPOINT / MINIO_USER / MINIO_PASSWORD)
```

## 1. Private bucket + versioning + lifecycle

A private bucket that keeps versions and expires objects after 30 days (and old
versions after 7).
```hcl
resource "minio_s3_bucket" "logs" {
  bucket = "app-logs"
  acl    = "private"
}

resource "minio_s3_bucket_versioning" "logs" {
  bucket = minio_s3_bucket.logs.bucket
  versioning_configuration {
    status = "Enabled"
  }
}

resource "minio_s3_bucket_lifecycle" "logs" {
  bucket = minio_s3_bucket.logs.bucket
  rule {
    id     = "expire-old-logs"
    status = "Enabled"
    expiration { days = 30 }
    noncurrent_version_expiration { noncurrent_days = 7 }
  }
}
```

## 2. Public (anonymous-read) bucket

Exposes every object to the world — confirm the user really wants this.
```hcl
resource "minio_s3_bucket" "assets" {
  bucket = "public-assets"
}

resource "minio_s3_bucket_anonymous_access" "assets" {
  bucket      = minio_s3_bucket.assets.bucket
  access_type = "public-read"   # readable by anyone; objects are still uploaded privately by you
}
```
Use `minio_s3_bucket_policy` instead if you need custom/conditional public rules.
Don't put both on one bucket.

## 3. Read-only user for a bucket

The canonical permissions pattern: build the policy with a data source, create a
managed policy, attach it to a user. Note the bucket needs **two** ARNs
(`bucket` for `ListBucket`, `bucket/*` for object reads).
```hcl
data "minio_iam_policy_document" "reports_ro" {
  statement {
    sid       = "ListBucket"
    actions   = ["s3:ListBucket"]
    resources = ["arn:aws:s3:::reports"]
  }
  statement {
    sid       = "ReadObjects"
    actions   = ["s3:GetObject"]
    resources = ["arn:aws:s3:::reports/*"]
  }
}

resource "minio_iam_policy" "reports_ro" {
  name   = "reports-read-only"
  policy = data.minio_iam_policy_document.reports_ro.json
}

resource "minio_iam_user" "auditor" {
  name = "auditor"
}

resource "minio_iam_user_policy_attachment" "auditor_ro" {
  user_name   = minio_iam_user.auditor.id   # user_name / policy_name (not user/policy)
  policy_name = minio_iam_policy.reports_ro.id
}
```

## 4. App credentials via service account

When an application needs an access/secret key pair, prefer a service account
over the user's own secret. A service account **cannot exceed its parent user's
permissions**, so the user must hold the permissions (here via an attached
policy); the SA inherits them.
```hcl
data "minio_iam_policy_document" "uploader" {
  statement {
    sid       = "ListMediaPrefix"
    actions   = ["s3:ListBucket"]
    resources = ["arn:aws:s3:::media"]
    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = ["uploads/*"]
    }
  }
  statement {
    sid       = "ReadWriteUploads"
    actions   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
    resources = ["arn:aws:s3:::media/uploads/*"]
  }
}

resource "minio_iam_policy" "uploader" {
  name   = "media-uploader"
  policy = data.minio_iam_policy_document.uploader.json
}

resource "minio_iam_user" "app" {
  name = "media-app"
}

resource "minio_iam_user_policy_attachment" "app" {
  user_name   = minio_iam_user.app.id
  policy_name = minio_iam_policy.uploader.id
}

resource "minio_iam_service_account" "app" {
  target_user = minio_iam_user.app.name
  # optionally narrow further with: policy = data.minio_iam_policy_document.<even_tighter>.json
}

output "app_access_key" {
  value = minio_iam_service_account.app.access_key
}
output "app_secret_key" {
  value     = minio_iam_service_account.app.secret_key
  sensitive = true   # read it with: terraform output -raw app_secret_key
}
```
If the secret must never touch state, use `minio_accesskey` with a write-only
secret from a secret store instead (see `resources.md`).

## 5. Group + managed policy + membership

```hcl
resource "minio_iam_group" "devs" {
  name = "developers"
}

resource "minio_iam_policy" "dev" {
  name   = "dev-access"
  policy = data.minio_iam_policy_document.dev.json
}

resource "minio_iam_group_policy_attachment" "dev" {
  group_name  = minio_iam_group.devs.id   # group_name / policy_name
  policy_name = minio_iam_policy.dev.id
}

resource "minio_iam_group_membership" "devs" {
  name  = "developers-members"
  group = minio_iam_group.devs.name
  users = [minio_iam_user.alice.name, minio_iam_user.bob.name]
}
```

## 6. Bucket replication

Source bucket must be versioned; the target bucket must already exist and be
versioned on the destination server. Target credentials are sensitive.
```hcl
resource "minio_s3_bucket" "src" {
  bucket = "data-primary"
}

resource "minio_s3_bucket_versioning" "src" {
  bucket = minio_s3_bucket.src.bucket
  versioning_configuration { status = "Enabled" }
}

resource "minio_s3_bucket_replication" "src" {
  bucket = minio_s3_bucket.src.bucket
  rule {
    enabled  = true
    priority = 1
    target {
      bucket     = "data-dr"
      host       = "minio-dr.example.com:9000"
      access_key = var.dr_access_key
      secret_key = var.dr_secret_key   # sensitive
      secure     = true
    }
  }
}
```

## 7. Server-side encryption

SSE-S3 (`AES256`) needs no external key:
```hcl
resource "minio_s3_bucket_server_side_encryption" "secure" {
  bucket          = minio_s3_bucket.secure.bucket
  encryption_type = "AES256"
}
```
For SSE-KMS, set `encryption_type = "aws:kms"` and `kms_key_id = "<existing-key>"`.
The key must already exist on the server (create/lookup via `minio_kms_key` or
`mc admin kms key`). MinIO KMS must be configured (KES) for this to work.

## 8. Import existing resources

Prefer the reviewable `import {}` block (Terraform ≥ 1.5) so the import shows up
in a plan before it touches state:
```hcl
import {
  to = minio_s3_bucket.legacy
  id = "legacy-bucket"
}

resource "minio_s3_bucket" "legacy" {
  bucket = "legacy-bucket"
}
```
Then `terraform plan` (optionally `-generate-config-out=generated.tf` to scaffold
the resource block). Or the CLI form for one-offs:
```bash
terraform import minio_iam_user.alice alice
terraform import minio_iam_user_policy_attachment.x alice/reports-read-only   # user/policy
terraform import minio_s3_object.f my-bucket/path/file.txt                    # bucket/object
```
Import IDs for every resource are tabulated in `resources.md` → *Import syntax*.
After importing, run `terraform plan` and reconcile any diff (some resources, e.g.
objects and lifecycle, only read partial state on import).
