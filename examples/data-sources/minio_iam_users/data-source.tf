data "minio_iam_users" "all_enabled" {
  status = "enabled"
}

output "enabled_user_count" {
  value = length(data.minio_iam_users.all_enabled.users)
}

output "enabled_user_names" {
  value = [for user in data.minio_iam_users.all_enabled.users : user.name]
}

data "minio_iam_users" "service_accounts" {
  name_prefix = "svc-"
  status      = "enabled"
}

output "service_account_users" {
  value = data.minio_iam_users.service_accounts.users
}

data "minio_iam_users" "all" {
  status = "all"
}

output "total_users" {
  value = length(data.minio_iam_users.all.users)
}

resource "minio_iam_user_policy_attachment" "readonly_for_all" {
  for_each = {
    for user in data.minio_iam_users.all_enabled.users :
    user.name => user
  }

  user_name   = each.key
  policy_name = "readonly-policy"
}
