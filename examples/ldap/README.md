# MinIO LDAP Integration Example

This example demonstrates how to use the MinIO Terraform provider with LDAP authentication.

## Prerequisites

1. MinIO server configured with LDAP authentication
2. LDAP server with users and groups
3. MinIO admin credentials

## MinIO LDAP Configuration

Before using these Terraform resources, MinIO must be configured to use LDAP.
This is typically done via environment variables or `mc admin config`:

```bash
mc admin config set myminio identity_ldap \
   server_addr="ldap.example.com:636" \
   lookup_bind_dn="cn=admin,dc=example,dc=com" \
   lookup_bind_password="admin-password" \
   user_dn_search_base_dn="ou=users,dc=example,dc=com" \
   user_dn_search_filter="(uid=%s)" \
   group_search_base_dn="ou=groups,dc=example,dc=com" \
   group_search_filter="(&(objectclass=groupOfNames)(member=%d))" \
   tls_skip_verify="off"
```

## Usage

1. Set your variables:

```bash
export TF_VAR_minio_server="minio.example.com:9000"
export TF_VAR_minio_access_key="minioadmin"
export TF_VAR_minio_secret_key="minioadmin"
```

2. Initialize and apply:

```bash
terraform init
terraform plan
terraform apply
```

## Important Notes

- The `group_dn` and `user_dn` must exactly match the Distinguished Names in your LDAP directory
- LDAP users authenticate using their LDAP credentials, not MinIO credentials
- Policies attached via these resources take effect immediately for LDAP users

## Resources

- [MinIO LDAP Documentation](https://min.io/docs/minio/linux/operations/external-iam/configure-ad-ldap-external-identity-management.html)
- [MinIO IAM Policies](https://min.io/docs/minio/linux/administration/identity-access-management/policy-based-access-control.html)
