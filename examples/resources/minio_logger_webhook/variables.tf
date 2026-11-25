variable "splunk_token" {
  description = "Splunk HEC token used as the webhook auth token"
  type        = string
  sensitive   = true
}
