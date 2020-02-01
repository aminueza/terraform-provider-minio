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
      "s3:*",
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
      "s3:DeleteObject"
    ]

    resources = [
      "arn:aws:s3:::state-terraform-s3",
      "arn:aws:s3:::state-terraform-s3/*",
    ]
  }
}
resource "minio_iam_policy" "test_policy" {
  name = "state-terraform-s3"
  policy    = data.minio_iam_policy_document.example.json

}