variable "minio_region" {
  description = "Default MINIO region"
  default     = "us-east-1"
}

variable "minio_server" {
  description = "Default MINIO host and port"
  default = "localhost:9000"
}

variable "minio_user" {
  description = "MINIO user"
  default     = "minio"
}

variable "minio_password" {
  description = "MINIO password"
  default     = "minio123"
}