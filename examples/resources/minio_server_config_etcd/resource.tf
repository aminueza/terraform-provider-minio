resource "minio_server_config_etcd" "federation" {
  endpoints   = "http://etcd1:2379,http://etcd2:2379"
  path_prefix = "/minio"
}
