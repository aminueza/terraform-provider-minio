data "minio_kms_keys" "all" {}

data "minio_kms_keys" "terraform_managed" {
  pattern = "terraform-*"
}

output "key_names" {
  value = [for key in data.minio_kms_keys.all.keys : key.name]
}

output "terraform_managed_count" {
  value = length(data.minio_kms_keys.terraform_managed.keys)
}
