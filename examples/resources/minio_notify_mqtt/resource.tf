resource "minio_notify_mqtt" "iot" {
  name     = "iot"
  broker   = "tcp://mqtt.example.com:1883"
  topic    = "minio/events"
  username = "minio"
  password = var.mqtt_password
  qos      = 1
}
