variable "minio_server" {
  description = "MinIO server endpoint (host:port)"
  type        = string
}

variable "minio_access_key" {
  description = "MinIO access key"
  type        = string
}

variable "minio_secret_key" {
  description = "MinIO secret key"
  type        = string
  sensitive   = true
}

variable "minio_ssl" {
  description = "Enable SSL"
  type        = bool
  default     = true
}
