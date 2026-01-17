data "minio_iam_policy_document" "example" {
  statement {
    sid = "1"
    actions = [
      "s3:ListAllMyBuckets",
      "s3:GetBucketLocation",
    ]
    resources = [
      "arn:aws:s3:::*",
    ]
  }

  statement {
    actions = [
      "s3:ListBucket",
    ]
    resources = [
      "arn:aws:s3:::state-terraform-s3",
    ]
    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values = [
        "",
        "home/",
      ]
    }
  }

  statement {
    actions = [
      "s3:PutObject",
    ]
    resources = [
      "arn:aws:s3:::state-terraform-s3",
      "arn:aws:s3:::state-terraform-s3/*",
    ]
  }
}

# Example using not_resources to allow access to all buckets except secrets
data "minio_iam_policy_document" "allow_all_except_secrets" {
  statement {
    sid    = "AllowAllExceptSecrets"
    effect = "Allow"
    
    actions = ["s3:*"]
    
    not_resources = [
      "arn:aws:s3:::secrets-bucket",
      "arn:aws:s3:::secrets-bucket/*",
      "arn:aws:s3:::confidential-bucket",
      "arn:aws:s3:::confidential-bucket/*",
    ]
    
    principal = "*"
  }
}

resource "minio_iam_policy" "test_policy" {
  name      = "state-terraform-s3"
  policy    = data.minio_iam_policy_document.example.json
}

resource "minio_iam_policy" "allow_all_except_secrets_policy" {
  name      = "allow-all-except-secrets"
  policy    = data.minio_iam_policy_document.allow_all_except_secrets.json
}