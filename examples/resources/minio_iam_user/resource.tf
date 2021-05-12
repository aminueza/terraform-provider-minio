resource "minio_iam_user" "test" {
   name = "test"
   force_destroy = true
   tags = {
    tag-key = "tag-value"
  }
}

output "test" {
  value = "${minio_iam_user.test.id}"
}

output "status" {
  value = "${minio_iam_user.test.status}"
}

output "secret" {
  value = "${minio_iam_user.test.secret}"
}