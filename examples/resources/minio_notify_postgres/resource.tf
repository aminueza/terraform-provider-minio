resource "minio_notify_postgres" "events" {
  name              = "primary"
  connection_string = "host=postgres.example.com port=5432 dbname=minio_events user=minio password=secret sslmode=disable"
  table             = "bucket_events"
  format            = "namespace"
}
