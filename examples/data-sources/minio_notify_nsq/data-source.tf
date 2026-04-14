data "minio_notify_nsq" "example" {
  name = "my-nsq"
}

output "nsq_topic" {
  value = data.minio_notify_nsq.example.topic
}
