resource "minio_batch_job" "expire" {
  job_type = "expire"
  job_yaml = <<-EOF
jobs:
  - name: expire-old-objects
    type: expire
    config:
      bucket: my-bucket
      prefix: temp/
      expire-days: 30
EOF
}
