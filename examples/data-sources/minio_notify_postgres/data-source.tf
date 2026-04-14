data "minio_notify_postgres" "example" {
  name = "my-postgres"
}

output "postgres_table" {
  value = data.minio_notify_postgres.example.table
}
