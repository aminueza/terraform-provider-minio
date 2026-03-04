resource "minio_notify_mysql" "events" {
  name              = "primary"
  connection_string = "user:password@tcp(mysql.example.com:3306)/minio_events"
  table             = "bucket_events"
  format            = "namespace"
}
