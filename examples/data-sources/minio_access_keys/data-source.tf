data "minio_access_keys" "builtin" {}

data "minio_access_keys" "everywhere" {
  identity_providers = ["builtin", "ldap", "openid"]
}

data "minio_access_keys" "one_user" {
  users = ["ci-deployer"]
}

output "keys_that_never_expire" {
  value = [
    for key in data.minio_access_keys.builtin.builtin : {
      access_key = key.access_key
      user       = key.parent_user
    }
    if key.expiration == ""
  ]
}

output "disabled_keys" {
  value = [
    for key in data.minio_access_keys.builtin.builtin : key.access_key
    if key.status != "on"
  ]
}

output "openid_login_credentials" {
  value = [
    for key in data.minio_access_keys.everywhere.openid : {
      access_key = key.access_key
      user       = key.readable_name
      config     = key.config_name
    }
    if key.kind == "sts"
  ]
}
