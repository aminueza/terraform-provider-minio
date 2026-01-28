data "minio_server_info" "current" {}

output "minio_version" {
  value = data.minio_server_info.current.version
}

output "minio_region" {
  value = data.minio_server_info.current.region
}

output "deployment_id" {
  value = data.minio_server_info.current.deployment_id
}

output "storage_summary" {
  value = {
    total_servers = length(data.minio_server_info.current.servers)
  }
}

output "servers" {
  value = data.minio_server_info.current.servers
}

output "all_servers_online" {
  value = alltrue([
    for server in data.minio_server_info.current.servers :
    server.state == "online"
  ])
}
