resource "minio_iam_idp_ldap" "default" {
  server_addr            = "ldap.example.com:636"
  lookup_bind_dn         = "cn=readonly,dc=example,dc=com"
  lookup_bind_password   = var.ldap_bind_password
  user_dn_search_base_dn = "ou=users,dc=example,dc=com"
  user_dn_search_filter  = "(uid=%s)"
  group_search_base_dn   = "ou=groups,dc=example,dc=com"
  group_search_filter    = "(&(objectclass=groupOfNames)(member=%d))"
}
