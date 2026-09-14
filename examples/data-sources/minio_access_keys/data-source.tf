data "minio_access_keys" "builtin" {}

data "minio_access_keys" "everywhere" {
  identity_providers = ["builtin", "ldap", "openid"]
}

data "minio_access_keys" "one_user" {
  users = ["ci-deployer"]
}

output "keys_terraform_does_not_manage" {
  value = [
    for key in data.minio_access_keys.builtin.builtin : key.access_key
    if key.description != "managed by terraform"
  ]
}

output "expiring_openid_keys" {
  value = [
    for key in data.minio_access_keys.everywhere.openid : {
      access_key = key.access_key
      user       = key.readable_name
      expires    = key.expiration
    }
    if key.expiration != ""
  ]
}
