resource "minio_server_config_browser" "main" {
  csp_policy              = "default-src 'self' https://console.example.com"
  hsts_seconds            = 31536000
  hsts_include_subdomains = true
  hsts_preload            = false
  referrer_policy         = "strict-origin-when-cross-origin"
}
