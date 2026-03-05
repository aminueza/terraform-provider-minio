data "minio_ilm_tiers" "available" {}

output "tier_names" {
  value = data.minio_ilm_tiers.available.tiers[*].name
}
