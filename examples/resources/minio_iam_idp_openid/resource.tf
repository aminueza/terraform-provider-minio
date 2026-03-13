resource "minio_iam_idp_openid" "default" {
  config_url    = "https://accounts.example.com/.well-known/openid-configuration"
  client_id     = "my-minio-client"
  client_secret = var.oidc_client_secret
  claim_name    = "policy"
  scopes        = "openid,email,profile"
  display_name  = "Example SSO"
}

# Named OIDC configuration (supports multiple providers)
resource "minio_iam_idp_openid" "keycloak" {
  name          = "keycloak"
  config_url    = "https://keycloak.example.com/realms/myrealm/.well-known/openid-configuration"
  client_id     = "minio"
  client_secret = var.keycloak_client_secret
  claim_name    = "policy"
  scopes        = "openid,email,profile"
  display_name  = "Keycloak"
  comment       = "Keycloak identity provider for MinIO"
}

ephemeral "vault_kv_secret_v2" "default_oidc_secret" {
  mount = "secret"
  name  = "minio/oidc/default"
}

ephemeral "vault_kv_secret_v2" "keycloak_oidc_secret" {
  mount = "secret"
  name  = "minio/oidc/keycloak"
}

# Write-only default (primary) OIDC configuration
resource "minio_iam_idp_openid" "default_write_only" {
  config_url               = "https://accounts.example.com/.well-known/openid-configuration"
  client_id                = "my-minio-client"
  client_secret_wo         = tostring(ephemeral.vault_kv_secret_v2.default_oidc_secret.data.client_secret)
  client_secret_wo_version = 1
  claim_name               = "policy"
  scopes                   = "openid,email,profile"
  display_name             = "Example SSO"
}

# Write-only named OIDC configuration (supports multiple providers)
resource "minio_iam_idp_openid" "keycloak_write_only" {
  name                     = "keycloak"
  config_url               = "https://keycloak.example.com/realms/myrealm/.well-known/openid-configuration"
  client_id                = "minio"
  client_secret_wo         = tostring(ephemeral.vault_kv_secret_v2.keycloak_oidc_secret.data.client_secret)
  client_secret_wo_version = 1
  claim_name               = "policy"
  scopes                   = "openid,email,profile"
  display_name             = "Keycloak"
  comment                  = "Keycloak identity provider for MinIO"
}
