# AI Coding Standards

Detailed conventions for implementing and modifying provider code. The short
version of the mandatory rules is in the repository root `AGENTS.md`.

## Comment Hygiene

Comments are the exception, not the rule: code should explain itself, and a comment is justified only when the code cannot. A comment must explain *why*, never *what*.

**Delete a comment when:**

- it restates the identifier or the statement below it (`// Set the ID to the key` above `d.SetId(key)`)
- it narrates self-evident steps (`// Initialize S3 client`, `// Parse the config`)
- it labels a group of entries that are already grouped by their own names (section markers in the resource/data-source maps)
- it is commented-out code (delete it; history lives in git)

**Keep a comment only when it records something the code cannot:**

- a non-obvious *why*: workarounds for MinIO server or SDK behavior, eventual-consistency handling, legacy/compatibility traps, state-convergence and security decisions
- contracts and edge cases not visible in the signature (return-value semantics, caller obligations, overflow/nil handling)
- references to external docs, issues, or tickets (e.g. `See issue #608`)

**Rules:**

- Never leave a stale comment: when you change the code a comment describes, update or delete the comment in the same change.
- Prefer deleting over trimming; when trimming, keep the *why* and drop the restatement.
- Doc comments on exported helpers are only warranted when they state behavior callers must know; the type/function name is not enough of a reason.

## Error Handling

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
```

**Validation errors (acceptable with `diag.Errorf`):** Pure input validation messages where the NewResourceError shape doesn't fit naturally:

```go
return diag.Errorf("bucket quota must be a non-negative value, got: %d", quotaInt)
return diag.Errorf("retention mode must be either GOVERNANCE or COMPLIANCE, got: %s", mode)
```

**Function signature:** `func minioCreateX(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics`

**Error types:** Handle `minio.ErrorResponse`, `madmin.ErrorResponse`, and generic errors. Diagnostics use proper severity levels. `NewResourceError` automatically extracts server response details when available.

**Pattern:**

```go
NewResourceError(operation string, resourceIdentifier string, err interface{}) diag.Diagnostics
```

- `operation`: gerund form ("creating bucket", "reading policy")
- `resourceIdentifier`: bucket/user/policy name

## Debug Logging

Use **`tflog`** (`github.com/hashicorp/terraform-plugin-log/tflog`), not `log.Printf`. `tflog` respects `TF_LOG_LEVEL` / `TF_LOG_PROVIDER`, is per-resource filterable, and supports structured fields.

```go
tflog.Debug(ctx, "Creating bucket", map[string]interface{}{"bucket": bucket})
tflog.Warn(ctx, fmt.Sprintf("missing %s, recreating", name))
tflog.Error(ctx, NewResourceErrorStr("op", id, err)) // pass strings directly; no fmt.Sprintf("%s", x)
```

Level mapping:

| Old | New |
|---|---|
| `log.Printf("[DEBUG] …")` | `tflog.Debug(ctx, …)` |
| `log.Printf("[INFO] …")` | `tflog.Info(ctx, …)` |
| `log.Printf("[WARN] …")` / `[WARNING]` | `tflog.Warn(ctx, …)` |
| `log.Printf("[ERROR] …")` | `tflog.Error(ctx, …)` |

Log at:

1. Start/end of Create
2. Start of Read
3. Start/end of Update
4. Start/end of Delete
5. Before critical API calls

Rules:

- `tflog` **requires `ctx`**. In ctx-less helpers, plumb `ctx` through rather than falling back to `log.Printf`.
- Prefer the `additionalFields map[string]interface{}` arg for structured key/values over `fmt.Sprintf` interpolation.
- Never wrap an already-string value in `fmt.Sprintf("%s", x)` — staticcheck S1025.
- Use concise, factual logs.

## Payload Pattern (mandatory)

Always:

1. Define the config struct in `minio/payload.go` (`S3Minio{ResourceName}`). Every type in the package is declared there; methods stay in the file that holds the logic, since Go only requires the type and its methods to share a package. `task lint` and CI both run `.github/scripts/check-type-placement.sh`, which fails on a `type` declaration anywhere else under `minio/`.
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

    tflog.Debug(ctx, "Creating object lock configuration", map[string]interface{}{"bucket": objectLockConfig.MinioBucket})

    if err := applyObjectLockConfiguration(ctx, d, objectLockConfig.MinioClient, objectLockConfig.MinioBucket); err != nil {
        return NewResourceError("applying object lock configuration", objectLockConfig.MinioBucket, err)
    }

    d.SetId(objectLockConfig.MinioBucket)
    tflog.Debug(ctx, "Created object lock configuration", map[string]interface{}{"bucket": objectLockConfig.MinioBucket})

    return minioReadObjectLockConfiguration(ctx, d, meta)
}
```

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

    tflog.Debug(ctx, "Creating X", map[string]interface{}{"name": config.MinioXName})

    if err := applyX(ctx, config); err != nil {
        return NewResourceError("creating X", config.MinioXName, err)
    }

    d.SetId(config.MinioXName)
    tflog.Debug(ctx, "Created X", map[string]interface{}{"name": config.MinioXName})

    return minioReadX(ctx, d, meta)
}
```

## CRUD Operations

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

- Always call Read after Create/Update (exception: skip Read when `restart_required=true` and the subsystem won't return the new values until restart).
- Always clear the ID (`d.SetId("")`) when Read determines the resource doesn't exist.
- Always set ID **after** the API call succeeds, not before. A failed create must not leave a phantom resource ID.
- Delete should be idempotent (treat not-found as success) and always call `d.SetId("")`.

## Schema Definitions

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

## Validation Patterns

Prefer built-in validators from `github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation`.

```go
ValidateFunc: validation.StringLenBetween(1, 63),
ValidateFunc: validation.StringInSlice([]string{"GOVERNANCE", "COMPLIANCE"}, false),
ValidateFunc: validation.IntAtLeast(1),
```

If you need a custom validator, keep it small and return actionable errors.

## Optional Fields and Type Conversions

Avoid panics from unchecked casts. For optional fields:

- Use `getOptionalField(d, "field", default)` when extracting into payload structs.
- Use `GetOk` with `HasChange` when you need presence semantics ("unset" vs "set to zero value"). **Do NOT use `GetOkExists`** — it is deprecated and triggers lint failures.

For collection types:

- `schema.TypeSet`: `d.Get("x").(*schema.Set).List()` and convert elements.
- `schema.TypeList`: `d.Get("x").([]interface{})` and convert elements.

## Integer Conversion Safety

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

## MinIO SDK Notes

- **Client creation:** Use `newMinioClient()` from `new_client.go` for consistent client setup
- **Passing headers on uploads:** `minio.Client.PutObject()` uses `minio.PutObjectOptions.Header()` to build request headers
- **`UserMetadata` behavior:** keys in `PutObjectOptions.UserMetadata` that look like S3/MinIO headers are sent as-is; other keys are prefixed with `x-amz-meta-`
- **Object ACLs:** to set a canned ACL during upload, set `PutObjectOptions.UserMetadata["x-amz-acl"] = "<acl>"` (e.g. `public-read`)
- **Acceptance tests:** `go test` skips acceptance tests unless `TF_ACC` is set. The Docker Compose test runner (`docker compose run --rm test`) is the preferred way to execute acceptance tests
- **Bulk deletion:** Use `minio.Client.RemoveObjects()` with a channel of `ObjectInfo` for efficient bulk deletion. Always drain the error channel to collect failures
- **Listing with versions:** Use `ListObjectsOptions{WithVersions: true}` to include all object versions and delete markers when deleting versioned buckets

## Error Recovery and Idempotency

- Prefer read-before-create patterns when the API supports it.
- Make Delete safe to retry (not-found should not error).
- If Create does multiple steps, clean up partial state on failure when feasible.

## Performance Considerations

- Prefer bulk APIs where available (e.g., `RemoveObjects`) and always drain error channels.
- Use pagination/streaming APIs instead of loading everything into memory.
- Avoid N+1 patterns (don't issue per-object calls when a list API can return enough data).

## Import Organization

Group imports as:

1. Standard library (alphabetical)
2. Third-party
3. Internal

## Naming Conventions

- `resourceMinio{ResourceName}()` for resource definitions
- `minioCreate{ResourceName}` / `minioRead{ResourceName}` / `minioUpdate{ResourceName}` / `minioDelete{ResourceName}` for CRUD
- `S3Minio{ResourceName}` for payload structs
- `{ResourceName}Config` for extractors

## Provider Registration

Always register new resources in `minio/provider.go`:

```go
ResourcesMap: map[string]*schema.Resource{
    "minio_{resource_name}": resourceMinio{ResourceName}(),
},
```

## Testing Standards

- Naming: `TestAcc{ResourceName}_{scenario}`
- Always use random names: `"tfacc-resource-" + acctest.RandString(8)`
- Minimum tests: `_basic`, `_update` (import test recommended)
- Every entry in `testAccProviders` must return a fresh `newProvider(...)` on each call. Tests run in parallel and configure the provider they are handed, so a shared instance lets one test's credentials reach another's. Check helpers get their client from `testAccClient()` (or the `testAccSecondClient`/`Third`/`Fourth`/`Kms`/`Ldap` variants), never from a provider a test configured.

## Checklist for New Resources

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
- Template file created: `templates/resources/{name}.md.tmpl`
- Example TF file created: `examples/resources/minio_{name}/resource.tf`
- Test file created: `minio/resource_minio_{name}_test.go`

## Checklist for New Data Sources

- Data source function defined: `dataSourceMinio{Name}()`
- Read function implemented with correct naming
- All errors use `NewResourceError`
- Schema: `bucket` (or lookup key) Required, all other fields Computed
- Data source registered in `provider.go` `DataSourcesMap`
- Template file created: `templates/data-sources/{name}.md.tmpl`
- Example TF file created: `examples/data-sources/minio_{name}/data-source.tf`
- Test file created: `minio/data_source_minio_{name}_test.go`
- Acceptance test covers at least `_basic` scenario
