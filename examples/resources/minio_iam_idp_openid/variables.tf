variable "oidc_client_secret" {
  description = "Client secret of the default OpenID Connect provider"
  type        = string
  sensitive   = true
}

variable "keycloak_client_secret" {
  description = "Client secret of the Keycloak OpenID Connect provider"
  type        = string
  sensitive   = true
}
