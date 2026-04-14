data "minio_notify_mysql" "example" {
  name = "my-mysql"
}

output "mysql_table" {
  value = data.minio_notify_mysql.example.table
}
