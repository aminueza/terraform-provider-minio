


resource "minio_iam_group" "developer" {
  name = "developer"
  force_destroy = true
}