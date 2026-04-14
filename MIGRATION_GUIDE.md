# Migration Guide: v3 to v4

This guide helps you migrate from terraform-provider-minio v3 (SDK-based) to v4 (Framework-based).

## Overview

Version 4.0 is a major release that migrates the provider from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. This provides:

- Better type safety
- Improved diagnostics
- Modern Terraform provider architecture
- Better support for complex configurations

## Breaking Changes

### Replication Resources Re-implemented

The following resources have been re-implemented in v4.0 with updated MinIO SDK APIs:

- `minio_s3_bucket_replication` - Bucket replication configuration
- `minio_site_replication` - Site replication configuration

**API Changes**:
- Bucket replication now properly uses remote targets via admin API
- Filter structure changed: `S3Key` replaced with `Tag` and `And` fields
- Delete replication uses `Status` field instead of `ReplicateDelete()` method
- Destination `HealthCheck` and `BandwidthLimit` fields removed (managed via admin API)

**Migration**: If you're using these resources, review the updated API structure. Existing configurations may need minor adjustments to work with the new implementation.

### No Other Breaking Changes

All other resources maintain backward compatibility. Your existing v3 configurations will work with v4 with no changes required.

## Migration Steps

### 1. Update Provider Configuration

Update your provider version constraint:

```hcl
terraform {
  required_providers {
    minio = {
      source  = "aminueza/minio"
      version = "~> 4.0"
    }
  }
}
```

### 2. Review Replication Resources

If you use `minio_s3_bucket_replication` or `minio_site_replication`:

**Review API Changes**: The replication resources have been re-implemented with updated SDK APIs. Check the documentation for:
- Updated filter structure for bucket replication
- Proper remote target configuration
- Bandwidth limit and health check settings via admin API

### 3. Run Terraform Plan

```bash
terraform plan
```

Verify that:
- All resources are properly managed
- Replication resources show any necessary changes due to API updates

### 4. Apply Changes

```bash
terraform apply
```

## State Management

No state migration is required. Terraform will automatically detect the provider version change and continue managing existing resources.

## Testing

After migration, verify your infrastructure:

```bash
# Verify all resources are managed
terraform state list

# Check for any drift
terraform plan
```

## Rollback

If you need to rollback to v3:

```hcl
terraform {
  required_providers {
    minio = {
      source  = "aminueza/minio"
      version = "~> 3.0"
    }
  }
}
```

```bash
terraform init -upgrade
```

## v4.1 Roadmap

The following resources will be re-implemented in v4.1:

- `minio_s3_bucket_replication` - Bucket replication with updated API
- `minio_site_replication` - Site replication with updated API

## Support

For issues or questions:
- [GitHub Issues](https://github.com/aminueza/terraform-provider-minio/issues)
- [Discussion Forum](https://github.com/aminueza/terraform-provider-minio/discussions)

## Changelog

### v4.0.0

**Breaking Changes:**
- Migrated from terraform-plugin-sdk to terraform-plugin-framework
- Removed `minio_s3_bucket_replication` resource (API changes)
- Removed `minio_site_replication` resource (API changes)

**Enhancements:**
- 70+ resources now use terraform-plugin-framework
- Improved error handling and diagnostics
- Better type safety
- Modern provider architecture

### v3.x

Previous SDK-based version. Continue using for replication resources until v4.1.
