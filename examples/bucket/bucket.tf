resource "minio_s3_bucket" "state_terraform_s3" {
  bucket = "state-terraform-s3"
  acl    = "public"
}

resource "minio_s3_object" "txt_file" {
  depends_on  = [minio_s3_bucket.state_terraform_s3]
  bucket_name = minio_s3_bucket.state_terraform_s3.bucket
  object_name = "text.txt"
  content     = "Lorem ipsum dolor sit amet."
}

resource "minio_s3_object" "png_file" {
  depends_on     = [minio_s3_bucket.state_terraform_s3]
  bucket_name    = minio_s3_bucket.state_terraform_s3.bucket
  object_name    = "image.png"
  content_base64 = "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"
}

resource "minio_s3_bucket_policy" "policy" {
  depends_on = [minio_s3_bucket.state_terraform_s3]
  bucket     = minio_s3_bucket.state_terraform_s3.bucket
  policy     = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {"AWS": ["*"]},
      "Resource": ["${minio_s3_bucket.state_terraform_s3.arn}"],
      "Action": ["s3:ListBucket"]
    }
  ]
}
EOF
}

resource "minio_s3_bucket_versioning" "bucket" {
  depends_on = [minio_s3_bucket.state_terraform_s3]
  bucket     = minio_s3_bucket.state_terraform_s3.bucket

  versioning_configuration {
    status = "Enabled"
  }
}

resource "minio_s3_bucket_notification" "bucket" {
  bucket = minio_s3_bucket.state_terraform_s3.bucket

  queue {
    id        = "notification-queue"
    queue_arn = "arn:minio:sqs::primary:webhook"

    events = [
      "s3:ObjectCreated:*",
      "s3:ObjectRemoved:Delete",
    ]

    filter_prefix = "example/"
    filter_suffix = ".png"
  }
}
