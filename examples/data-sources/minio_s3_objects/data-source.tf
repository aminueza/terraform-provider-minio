data "minio_s3_objects" "logs" {
  bucket    = "my-bucket"
  prefix    = "logs/2026/"
  max_keys  = 100
}

output "log_files" {
  value = data.minio_s3_objects.logs.keys
}

# Browse like a filesystem with delimiter
data "minio_s3_objects" "dirs" {
  bucket    = "my-bucket"
  prefix    = ""
  delimiter = "/"
}

output "top_level_dirs" {
  value = data.minio_s3_objects.dirs.common_prefixes
}
