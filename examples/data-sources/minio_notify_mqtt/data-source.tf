data "minio_notify_mqtt" "example" {
  name = "my-mqtt"
}

output "mqtt_broker" {
  value = data.minio_notify_mqtt.example.broker
}
