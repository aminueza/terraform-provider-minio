# Route authorization decisions to Open Policy Agent
resource "minio_server_config_policy_plugin" "opa" {
  url          = "http://opa:8181/v1/data/httpapi/authz"
  auth_token   = "[REDACTED:generic-assignment]"
  enable_http2 = true
  comment      = "Policy decisions are made by OPA"
}
