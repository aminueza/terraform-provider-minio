# resource "minio_bucket" "state_terraform_s3" {
#   bucket = "state-terraform-s3"
#   acl    = "public"
#   debug = false
# }

# resource "minio_iam_user" "maria" {
#   name = "maria"
#   force_destroy = true
# }

# output "user_maria" {
#   value = "${minio_iam_user.maria.id}"
# }

# output "status" {
#   value = "${minio_iam_user.maria.status}"
# }

# output "secret" {
#   value = "${minio_iam_user.maria.secret}"
# }

