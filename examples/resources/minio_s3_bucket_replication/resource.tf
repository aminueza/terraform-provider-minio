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
      bandwidth_limt = "100M"
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
      bandwidth_limt = "100M"
      access_key = minio_iam_service_account.replication_in_a.access_key
      secret_key = minio_iam_service_account.replication_in_a.secret_key
    }
  }

  depends_on = [
    minio_s3_bucket_versioning.my_bucket_in_a,
    minio_s3_bucket_versioning.my_bucket_in_b,
  ]
}