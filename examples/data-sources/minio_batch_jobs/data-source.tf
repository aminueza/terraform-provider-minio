data "minio_batch_jobs" "all" {}

data "minio_batch_jobs" "expire" {
  job_type = "expire"
}
