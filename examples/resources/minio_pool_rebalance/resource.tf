resource "minio_pool_rebalance" "example" {
  triggers = {
    reason = "add-drive"
  }
}
