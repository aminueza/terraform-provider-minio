---
description: |-
  Provides a MinIO Site Replication resource. This allows you to configure and manage site replication across multiple MinIO clusters, synchronizing buckets, IAM policies, users, groups, and other configurations.

  Site replication is different from bucket replication:
  - **Bucket Replication**: Replicates objects from one bucket to another (possibly on different clusters)
  - **Site Replication**: Synchronizes entire MinIO deployments including buckets, objects, metadata, IAM policies, users, groups, bucket policies, tags, and ILM rules

---

# `minio_site_replication`

## Example Usage

```hcl
resource "minio_site_replication" "primary" {
  name = "my-cluster-replication"

  site {
    name       = "site1"
    endpoint   = "https://minio1.example.com"
    access_key = "minioadmin"
    secret_key = "minio123"
  }

  site {
    name       = "site2"
    endpoint   = "https://minio2.example.com"
    access_key = "minioadmin"
    secret_key = "minio456"
  }

  site {
    name       = "site3"
    endpoint   = "https://minio3.example.com"
    access_key = "minioadmin"
    secret_key = "minio789"
  }
}
```

## Argument Reference

* `name` - (Required, Forces new resource) Name of the site replication configuration.
* `site` - (Required) List of sites to replicate between. Minimum 2 sites required.
  * `name` - (Required) Unique name for this site.
  * `endpoint` - (Required) MinIO server endpoint URL.
  * `access_key` - (Required) Access key for the site. This value is stored in Terraform state but not returned by the MinIO API for security reasons.
  * `secret_key` - (Required, Sensitive) Secret key for the site. This value is stored in Terraform state but not returned by the MinIO API for security reasons.
* `enabled` - (Computed) Whether site replication is enabled.

## Attributes Reference

* `id` - The name of the site replication configuration.
* `name` - The name of the site replication configuration.
* `enabled` - Whether site replication is enabled.
* `site` - List of configured sites (only contains name and endpoint, as credentials are not returned by the API for security reasons).

## Import

Import is supported using the site replication name:

```shell
terraform import minio_site_replication.primary my-cluster-replication
```

## Notes

- **Security**: Access keys and secret keys are stored in Terraform state but are not returned by the MinIO API during read operations for security reasons.
- **Minimum Sites**: At least 2 sites are required for site replication to function.
- **Cluster-wide**: Site replication is a cluster-wide configuration that affects the entire MinIO deployment.
- **Dependencies**: All sites must be reachable from each other and have proper network connectivity.
- **Credentials**: Ensure that the access keys used have sufficient permissions to manage replication across all sites.

## Troubleshooting

### Common Issues

1. **"Global deployment ID mismatch"**: This error occurs when there are conflicting site replication configurations. Ensure that only one site replication configuration exists per cluster.
2. **"Unable to fetch server info"**: Check network connectivity and ensure endpoints are accessible from all sites.
3. **Permission errors**: Verify that the access keys have sufficient administrative permissions on all target sites.

### Debug Commands

```bash
# Check current site replication status
mc admin replicate info <alias>

# Remove existing site replication (if needed)
mc admin replicate remove <alias> --all

# Test connectivity between sites
mc admin replicate add <alias> <peer-site-endpoint> --access-key <key> --secret-key <secret> --dry-run
```
