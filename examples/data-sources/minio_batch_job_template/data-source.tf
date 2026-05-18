data "minio_batch_job_template" "replicate" {
  job_type = "replicate"
}

resource "minio_batch_job" "migrate" {
  job_type = "replicate"
  job_yaml = data.minio_batch_job_template.replicate.yaml
}
