# Framework Migration Progress

## Overview
Migration from terraform-plugin-sdk/v2 to terraform-plugin-framework using terraform-plugin-mux.

## Status

### Completed (3 resources, 1 data source)
- ✅ `minio_s3_bucket` - Bucket management
- ✅ `minio_iam_user` - IAM user management with secret rotation
- ✅ `minio_iam_policy` - IAM policy management
- ✅ `minio_s3_bucket` (data source) - Bucket data source

### Remaining Resources (57)

#### IAM Resources (15)
- [ ] `minio_iam_group` - IAM group management
- [ ] `minio_iam_group_membership` - Group membership management
- [ ] `minio_iam_group_policy` - Group policy attachment
- [ ] `minio_iam_group_policy_attachment` - Group policy attachment (simplified)
- [ ] `minio_iam_service_account` - Service account management
- [ ] `minio_iam_user_policy_attachment` - User policy attachment
- [ ] `minio_iam_group_user_attachment` - Group-user attachment
- [ ] `minio_iam_ldap_user_policy_attachment` - LDAP user policy attachment
- [ ] `minio_iam_ldap_group_policy_attachment` - LDAP group policy attachment
- [ ] `minio_iam_user_policy_attachment` - User policy attachment
- [ ] (more IAM resources...)

#### S3 Bucket Resources (10)
- [ ] `minio_s3_bucket_policy` - Bucket policy
- [ ] `minio_s3_bucket_versioning` - Bucket versioning
- [ ] `minio_s3_bucket_server_side_encryption_configuration` - Bucket encryption
- [ ] `minio_s3_bucket_object_lock_configuration` - Object lock
- [ ] `minio_s3_bucket_replication_configuration` - Bucket replication
- [ ] `minio_s3_bucket_notification` - Bucket notifications
- [ ] `minio_s3_bucket_anonymous_access` - Anonymous access control
- [ ] (more bucket resources...)

#### S3 Object Resources (3)
- [ ] `minio_s3_object` - Object management
- [ ] `minio_s3_object_lock` - Object lock (separate resource)
- [ ] (more object resources...)

#### ILM/Tier Resources (8)
- [ ] `minio_ilm_tier` - ILM tier management
- [ ] `minio_ilm_policy` - ILM policy management
- [ ] (more ILM resources...)

#### Configuration Resources (15+)
- [ ] `minio_config_kvs` - Configuration key-value stores
- [ ] Various `notify_*` resources - Notification targets
- [ ] Various `logger_*` resources - Logging configuration
- [ ] Various `audit_*` resources - Audit configuration
- [ ] (more config resources...)

### Remaining Data Sources (31)
- [ ] `minio_iam_user` - IAM user data source
- [ ] `minio_iam_group` - IAM group data source
- [ ] `minio_iam_policy` - IAM policy data source
- [ ] `minio_s3_bucket` (multiple variants)
- [ ] (more data sources...)

## Migration Pattern

All framework resources follow this pattern:

1. **Resource struct** - Implements `resource.Resource` interface
2. **Model struct** - Terraform state data model with `tfsdk` tags
3. **Metadata** - Returns resource type name
4. **Schema** - Defines attributes with validators and plan modifiers
5. **Configure** - Sets up client connection
6. **Create/Read/Update/Delete** - CRUD operations
7. **ImportState** - Import support
8. **Constructor** - `new<Resource>()` function

## Key Differences from SDK

| SDK | Framework |
|-----|-----------|
| `schema.ResourceData` | `types.String`, `types.Bool`, etc. |
| `diag.Diagnostics` | `diag.Diagnostics` (similar) |
| `ForceNew: true` | `stringplanmodifier.RequiresReplace()` |
| `Computed: true` | Same, but with type system |
| `ValidateFunc` | `validator.String`, `validator.Int64`, etc. |
| `CustomizeDiff` | `planmodifier` |

## Benefits of Framework

- Better type safety
- Improved error handling
- Modern Terraform features support
- Better testing capabilities
- Future-proof (SDK is in maintenance mode)

## Next Steps

1. Continue migrating resources in priority order
2. Add acceptance tests for each migrated resource
3. Update documentation templates
4. Verify all existing tests pass with mux setup
5. Plan deprecation timeline for SDK resources

## Testing

Run tests with:
```bash
docker compose run --rm test
```

Or specific tests:
```bash
TEST_PATTERN=TestAccMinioIAMUser docker compose run --rm test
```

## Build Verification

```bash
go build ./...
```
