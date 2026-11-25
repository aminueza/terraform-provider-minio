variable "aws_access_key" {
  description = "Access key of the S3-compatible remote tier"
  type        = string
  sensitive   = true
}

variable "aws_secret_key" {
  description = "Secret key of the S3-compatible remote tier"
  type        = string
  sensitive   = true
}

variable "remote_access_key" {
  description = "Access key of the remote MinIO tier"
  type        = string
  sensitive   = true
}

variable "remote_secret_key" {
  description = "Secret key of the remote MinIO tier"
  type        = string
  sensitive   = true
}

variable "azure_account" {
  description = "Azure Blob Storage account name"
  type        = string
}

variable "azure_key" {
  description = "Azure Blob Storage account key"
  type        = string
  sensitive   = true
}

variable "gcs_credentials" {
  description = "Contents of the GCS service account JSON key file"
  type        = string
  sensitive   = true
}
