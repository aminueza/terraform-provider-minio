# Migration Guide: v3 to v4

This guide helps you migrate from terraform-provider-minio v3 (SDK-based) to v4 (Framework-based).

## Overview

Version 4.0 is a major release that migrates the provider from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. This provides:

- Better type safety
- Improved diagnostics
- Modern Terraform provider architecture
- Better support for complex configurations

## Breaking Changes

### Removed Resources

The following resources have been removed in v4.0 and will be re-implemented in v4.1:

- `minio_s3_bucket_replication` - Bucket replication configuration
- `minio_site_replication` - Site replication configuration

**Reason**: These resources rely on MinIO SDK APIs that have changed significantly. They will be re-implemented in v4.1 with updated API support.

**Workaround**: Use v3 for these specific resources until v4.1 is released.

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

### 2. Review Removed Resources

If you use `minio_s3_bucket_replication` or `minio_site_replication`:

**Option A**: Continue using v3 for these resources
```hcl
# Keep using v3 for replication resources
terraform {
  required_providers {
    minio = {
      source  = "aminueza/minio"
      version = "~> 3.0"
    }
  }
}
```

**Option B**: Remove replication configurations temporarily and re-add in v4.1

### 3. Run Terraform Plan

```bash
terraform plan
```

Verify that:
- No resources show as requiring replacement
- Only the removed replication resources show as needing to be handled

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
