# Repository Guidelines

A contributor guide for the `terraform-provider-minio` Terraform provider.

Terraform provider for MinIO object storage. Manages buckets, objects, IAM, ILM policies, encryption, replication, and related configuration using the MinIO S3 and Admin APIs.

## Quick Reference (read first)

**Error handling (mandatory):**

```go
if err != nil {
    return NewResourceError("creating bucket", bucketName, err)
}
```

**Never do this:**

```go
return diag.FromErr(err)
return diag.Errorf("error: %v", err)
```

**Payload pattern (mandatory):**

```go
config := ResourceNameConfig(d, meta)
```

**Always check `d.Set` errors:**

```go
if err := d.Set("field", value); err != nil {
    return NewResourceError("setting field", d.Id(), err)
}
```

**Safe integer conversions:**

```go
val64, ok := SafeUint64ToInt64(myUint64)
if !ok {
    return NewResourceError("converting value", d.Id(), fmt.Errorf("uint64 overflows int64: %d", myUint64))
}
```

**Schema field patterns:**

```go
"name": { Type: schema.TypeString, Required: true, ForceNew: true, Description: "Name" },
"enabled": { Type: schema.TypeBool, Optional: true, Default: true, Description: "Enable/disable" },
"id": { Type: schema.TypeString, Computed: true, Description: "Server-generated ID" },
```

**Hard guardrails (do not):**

- Never edit files in `docs/` directly (edit `templates/` and run `task generate-docs`).
- Never bypass `NewResourceError()` in resource/data source CRUD.
- Never print with `fmt.Println` / `println`.
- Never ignore errors from `d.Set` or API calls.
- Never hardcode test resource names (always randomize).

## Project Structure

```
├── main.go                 # Provider entry point
├── minio/                  # Core provider code
│   ├── provider.go         # Provider definition and resource registration
│   ├── resource_minio_*.go # Resource implementations
│   ├── data_source_*.go    # Data source implementations
│   ├── *_test.go           # Acceptance tests
│   ├── check_config.go     # Configuration helpers
│   ├── error.go            # Error handling utilities
│   ├── utils.go            # Common utilities and helpers
│   └── new_client.go       # MinIO client creation
├── templates/              # Documentation templates (.md.tmpl)
├── docs/                   # Generated documentation (do not edit directly)
├── examples/               # Example Terraform configurations
└── docker-compose.yml      # Test environment with multiple MinIO instances
```

## Build & Development Commands

| Command              | Description                                            |
| -------------------- | ------------------------------------------------------ |
| `go build ./...`     | Compile the provider                                   |
| `task build`         | Build provider binary to current directory             |
| `task install`       | Build and install to local Terraform plugins directory |
| `task test`          | Run all acceptance tests via Docker Compose            |
| `task generate-docs` | Regenerate documentation from templates                |
| `golangci-lint run`  | Run configured linters                                 |

**Run specific tests:**

```bash
# Run single test
TEST_PATTERN=TestAccMinioS3Bucket docker compose run --rm test

# Run tests with go directly (requires TF_ACC=1)
TF_ACC=1 go test -v ./minio -run TestAccMinioS3Bucket_basic

# Run package tests
go test ./minio/...
```

## Coding Style

- **Formatting:** Use `gofmt` (standard Go formatting)
- **Linting:** Configured via `.github/golangci.yml` with errcheck, govet, ineffassign, staticcheck, unused, bodyclose, noctx, unconvert
- **Imports:** Group standard library, external packages, then internal packages
- **Naming:**
  - Resources: `resource_minio_<name>.go` → `resourceMinio<Name>()`
  - Data sources: `data_source_minio_<name>.go` → `dataSourceMinio<Name>()`
  - Tests: `*_test.go` with `TestAcc<Resource>_<scenario>` functions
  - Private functions: camelCase starting with lowercase
  - Constants: UPPER_SNAKE_CASE
- **Types:** Use explicit types for all function parameters and return values
- **Error handling:** Use `NewResourceError()` from `error.go` for consistent diagnostics

## Error Handling Guidelines

- **Always use `NewResourceError()`** for resource operation errors
- **Function signature:** `func minioCreateX(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics`
- **Error types:** Handle `minio.ErrorResponse`, `madmin.ErrorResponse`, and generic errors
- **Diagnostics:** Return `diag.Diagnostics` with proper severity levels
- **Error details:** Include server response details when available (automatically extracted by `NewResourceError`)

## Testing Guidelines

- **Framework:** Terraform Plugin SDK acceptance tests
- **Environment:** Tests run against MinIO containers defined in `docker-compose.yml`
- **Naming:** `TestAcc<ResourceName>_<scenario>` (e.g., `TestAccMinioS3Bucket_basic`)
- **Requirements:** Each resource must have acceptance tests covering create, read, update, delete, and import
- **Test structure:** Use `resource.Test` with `TestCase` containing steps
- **PreCheck:** Implement `testAccPreCheck(t)` for provider configuration validation

```bash
# Run all tests
docker compose run --rm test

# Run specific test
TEST_PATTERN=TestAccMinioIAMUser docker compose run --rm test

# Run with verbose output
TF_ACC=1 go test -v ./minio -run TestAccMinioS3Bucket_basic
```

## Documentation

- Edit templates in `templates/`, not files in `docs/`.
- Run `task generate-docs` after template changes.
- Include example usage in each resource template.
- Keep examples referenced by templates in `examples/resources/<resource_name>/resource.tf` (used via `{{ tffile ... }}`).
- For LDAP policy attachment resources, the import ID format is `<distinguished-name>/<policy-name>` (DNs often contain commas, so quote the import string).
- LDAP resources attach policies only; MinIO LDAP configuration itself must be done outside Terraform (e.g. `mc admin config`).

**Template structure:** templates are `*.md.tmpl` files with frontmatter and a standard layout.

````tmpl
---
page_title: "{{.Name}} {{.Type}} - {{.ProviderName}}"
subcategory: ""
description: |-
{{ .Description | plainmarkdown | trimspace | prefixlines "  " }}
---

# {{.Name}} ({{.Type}})

{{ .Description | trimspace }}

## Example Usage

```terraform
{{ tffile "examples/resources/<resource_name>/resource.tf" }}
```

{{ .SchemaMarkdown | trimspace }}

## Import

...

````

**Template rules:**

- Put the user-facing documentation in the resource/data source `Description` in Go; `SchemaMarkdown` renders the schema.
- Put longer Terraform configuration examples in `examples/` and reference them from the template.
- Don’t manually edit generated docs; regenerate them.

## MinIO SDK Notes

- **Client creation:** Use `newMinioClient()` from `new_client.go` for consistent client setup
- **Passing headers on uploads:** `minio.Client.PutObject()` uses `minio.PutObjectOptions.Header()` to build request headers
- **`UserMetadata` behavior:** keys in `PutObjectOptions.UserMetadata` that look like S3/MinIO headers are sent as-is; other keys are prefixed with `x-amz-meta-`
- **Object ACLs:** to set a canned ACL during upload, set `PutObjectOptions.UserMetadata["x-amz-acl"] = "<acl>"` (e.g. `public-read`)
- **Acceptance tests:** `go test` skips acceptance tests unless `TF_ACC` is set. The Docker Compose test runner (`docker compose run --rm test`) is the preferred way to execute acceptance tests
- **Bulk object deletion:** Use `minio.Client.RemoveObjects()` with a channel of `ObjectInfo` for efficient bulk deletion. Always drain the error channel to collect failures
- **Listing with versions:** Use `ListObjectsOptions{WithVersions: true}` to include all object versions and delete markers when deleting versioned buckets

## Resource Implementation Pattern

```go
func resourceMinioX() *schema.Resource {
    return &schema.Resource{
        CreateContext: minioCreateX,
        ReadContext:   minioReadX,
        UpdateContext: minioUpdateX,
        DeleteContext: minioDeleteX,
        Importer: &schema.ResourceImporter{
            StateContext: schema.ImportStatePassthroughContext,
        },
        Schema: map[string]*schema.Schema{
            // Define schema here
        },
    }
}

func minioCreateX(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
    config := XConfig(d, meta)

    log.Printf("[DEBUG] Creating X: %s", config.MinioXName)

    if err := applyX(ctx, config); err != nil {
        return NewResourceError("creating X", config.MinioXName, err)
    }

    d.SetId(config.MinioXName)
    log.Printf("[DEBUG] Created X: %s", config.MinioXName)

    return minioReadX(ctx, d, meta)
}
```

## Commit & Pull Request Guidelines

**Commit messages:**

- Use **Conventional Commits** format: `type(scope): description`
- Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`
- Examples:
  - `feat(ilm): add support for abort-only rules`
  - `fix(bucket): handle missing lifecycle configuration`
  - `docs(s3): update object lock examples`

**Important:** Do NOT add yourself as a co-author of the commit.

**Pull requests:**

- Follow the template in `.github/PULL_REQUEST_TEMPLATE.md`
- Reference related issues (`Resolves #123`)
- Provide clear description of changes
- Ensure all tests pass
- Update documentation templates if adding/changing attributes
- Run `golangci-lint run` and fix any issues

---

## AI Coding Standards

### Error Handling

Always use `NewResourceError()` from `minio/error.go`.

**Correct:**

```go
if err != nil {
    return NewResourceError("creating bucket", bucketName, err)
}

if err := d.Set("name", name); err != nil {
    return NewResourceError("setting name", resourceID, err)
}
```

**Wrong:**

```go
return diag.FromErr(err)
return diag.Errorf("error: %v", err)
```

**Pattern:**

```go
NewResourceError(operation string, resourceIdentifier string, err interface{}) diag.Diagnostics
```

- `operation`: gerund form ("creating bucket", "reading policy")
- `resourceIdentifier`: bucket/user/policy name

### Debug Logging

Add `log.Printf("[DEBUG] ...")` at:

1. Start/end of Create
2. Start of Read
3. Start/end of Update
4. Start/end of Delete
5. Before critical API calls

Use concise, factual logs.

### Payload Pattern (mandatory)

Always:

1. Define the config struct in `minio/payload.go` (`S3Minio{ResourceName}`)
2. Define the extractor in `minio/check_config.go` (`{ResourceName}Config`)
3. Use the extracted config in CRUD (`config := ResourceNameConfig(d, meta)`)

**Concrete example (struct + extractor + usage):**

```go
// payload.go
type S3MinioBucketObjectLockConfiguration struct {
    MinioClient       *minio.Client
    MinioBucket       string
    ObjectLockEnabled string
    Mode              *minio.RetentionMode
    Validity          *uint
    Unit              *minio.ValidityUnit
}
```

```go
// check_config.go
// BucketObjectLockConfigurationConfig extracts object lock config from resource data.
func BucketObjectLockConfigurationConfig(d *schema.ResourceData, meta interface{}) *S3MinioBucketObjectLockConfiguration {
    m := meta.(*S3MinioClient)

    return &S3MinioBucketObjectLockConfiguration{
        MinioClient:       m.S3Client,
        MinioBucket:       getOptionalField(d, "bucket", "").(string),
        ObjectLockEnabled: getOptionalField(d, "object_lock_enabled", "Enabled").(string),
    }
}
```

```go
// resource CRUD
func minioCreateObjectLockConfiguration(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
    objectLockConfig := BucketObjectLockConfigurationConfig(d, meta)

    log.Printf("[DEBUG] Creating object lock configuration for bucket: %s", objectLockConfig.MinioBucket)

    if err := applyObjectLockConfiguration(ctx, d, objectLockConfig.MinioClient, objectLockConfig.MinioBucket); err != nil {
        return NewResourceError("applying object lock configuration", objectLockConfig.MinioBucket, err)
    }

    d.SetId(objectLockConfig.MinioBucket)
    log.Printf("[DEBUG] Created object lock configuration for bucket: %s", objectLockConfig.MinioBucket)

    return minioReadObjectLockConfiguration(ctx, d, meta)
}
```

### CRUD Operations

Follow this structure:

```
1. Extract config
2. Log DEBUG start
3. Validate
4. Apply operation with NewResourceError
5. Set ID (Create) or d.SetId("") (Delete)
6. Log DEBUG end
7. Return Read (Create/Update) or nil (Delete)
```

Key rules:

- Always call Read after Create/Update.
- Always clear the ID (`d.SetId("")`) when Read determines the resource doesn’t exist.
- Delete should be idempotent (treat not-found as success).

### Schema Definitions

Use clear Required/Optional/Computed intent and validate inputs.

**Required fields:**

```go
"field_name": {
    Type:         schema.TypeString,
    Required:     true,
    ForceNew:     true,
    ValidateFunc: validation.StringLenBetween(1, 63),
    Description:  "Short, clear description",
},
```

**Optional fields:**

```go
"field_name": {
    Type:        schema.TypeString,
    Optional:    true,
    Default:     "default-value",
    Description: "Short description",
},
```

**Computed fields:**

```go
"field_name": {
    Type:        schema.TypeString,
    Computed:    true,
    Description: "Short description",
},
```

**Nested blocks:**

```go
"rule": {
    Type:        schema.TypeList,
    Optional:    true,
    MaxItems:    1,
    Description: "Configuration block",
    Elem: &schema.Resource{ Schema: map[string]*schema.Schema{ ... } },
},
```

### Validation Patterns

Prefer built-in validators from `github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation`.

```go
ValidateFunc: validation.StringLenBetween(1, 63),
ValidateFunc: validation.StringInSlice([]string{"GOVERNANCE", "COMPLIANCE"}, false),
ValidateFunc: validation.IntAtLeast(1),
```

If you need a custom validator, keep it small and return actionable errors.

### Handling optional fields and type conversions

Avoid panics from unchecked casts. For optional fields:

- Use `getOptionalField(d, "field", default)` when extracting into payload structs.
- Use `GetOk`/`GetOkExists` when you need presence semantics ("unset" vs "set to zero value").

For collection types:

- `schema.TypeSet`: `d.Get("x").(*schema.Set).List()` and convert elements.
- `schema.TypeList`: `d.Get("x").([]interface{})` and convert elements.

### Integer conversion safety

Always check overflow when converting between `uint`/`int`/`uint64`/`int64`.

Wrong:

```go
days := int(*validity)
val := int64(myUint64)
```

Correct:

```go
import "math"

validityInt := int(*validity)
if *validity > uint(math.MaxInt) {
    validityInt = math.MaxInt
}

val64, ok := SafeUint64ToInt64(myUint64)
if !ok {
    return NewResourceError("converting value", d.Id(), fmt.Errorf("uint64 overflows int64: %d", myUint64))
}
```

### Error recovery and idempotency

- Prefer read-before-create patterns when the API supports it.
- Make Delete safe to retry (not-found should not error).
- If Create does multiple steps, clean up partial state on failure when feasible.

### Performance considerations

- Prefer bulk APIs where available (e.g., `RemoveObjects`) and always drain error channels.
- Use pagination/streaming APIs instead of loading everything into memory.
- Avoid N+1 patterns (don’t issue per-object calls when a list API can return enough data).

### Import organization

Group imports as:

1. Standard library (alphabetical)
2. Third-party
3. Internal

### Naming conventions

- `resourceMinio{ResourceName}()` for resource definitions
- `minioCreate{ResourceName}` / `minioRead{ResourceName}` / `minioUpdate{ResourceName}` / `minioDelete{ResourceName}` for CRUD
- `S3Minio{ResourceName}` for payload structs
- `{ResourceName}Config` for extractors

### Provider registration

Always register new resources in `minio/provider.go`:

```go
ResourcesMap: map[string]*schema.Resource{
    "minio_{resource_name}": resourceMinio{ResourceName}(),
},
```

### Testing Standards

- Naming: `TestAcc{ResourceName}_{scenario}`
- Always use random names: `"tfacc-resource-" + acctest.RandString(8)`
- Minimum tests: `_basic`, `_update` (import test recommended)

### Checklist for new resources

- Resource function defined: `resourceMinio{Name}()`
- CRUD functions implemented with correct naming
- Payload struct added to `payload.go`
- Config extractor added to `check_config.go`
- All CRUD operations use payload pattern
- All errors use `NewResourceError`
- Debug logs at start/end of operations
- Schema descriptions are clear and brief
- Resource registered in `provider.go`
- Acceptance tests include `_basic`, `_update`
- Integer conversions are safe

---

## Development Environment & Testing

### Test Environment Setup

**Available MinIO instances:**

- Primary: `localhost:9000` (minio/minio123)
- Second: `localhost:9002` (minio/minio321)
- Third: `localhost:9004` (minio/minio456)
- Fourth: `localhost:9006` (minio/minio654)

**Environment variables:**

```bash
export TF_ACC=1
export MINIO_ENDPOINT=localhost:9000
export MINIO_USER=minio
export MINIO_PASSWORD=minio123
export MINIO_ENABLE_HTTPS=false
```

### Running Tests

```bash
# Run all tests
docker compose run --rm test

# Run specific test
TEST_PATTERN=TestAccMinioIAMUser docker compose run --rm test

# Run with verbose output
TF_ACC=1 go test -v ./minio -run TestAccMinioS3Bucket_basic

# Run package tests
go test ./minio/...
```

### Common Issues

**ILM tier names:** Must be UPPERCASE:

```go
tierName := strings.ToUpper("TFACC-TIER-" + acctest.RandString(8))
```

**Object lock:** Requires versioning enabled. Needs MinIO RELEASE.2025-05-20+ to add after bucket creation.

**LDAP/KMS tests:** Skip automatically if not configured. Set `MINIO_LDAP_ENABLED=1` or `MINIO_KMS_ENABLED=1` to run.

## Reference Implementation

See `minio/resource_minio_s3_bucket_object_lock_configuration.go` as the reference implementation that follows ALL conventions in this document.

## Questions?

If unclear about any convention:

1. Check existing resources for patterns
2. Refer to `resource_minio_s3_bucket_object_lock_configuration.go` (newest, follows all conventions)
3. Check `resource_minio_s3_object.go` for mature patterns
4. Look at recent commits in git history
