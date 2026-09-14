# Repository Guidelines

A contributor guide for the `terraform-provider-minio` Terraform provider.

Terraform provider for MinIO object storage. Manages buckets, objects, IAM, ILM policies, encryption, replication, and related configuration using the MinIO S3 and Admin APIs. This file is the index; the detailed guides live in `docs/agents/` and are linked below.

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

**Every type is declared in `minio/payload.go` (mandatory):**

Methods stay in the file that holds the logic, since Go only requires the type
and its methods to share a package. `task lint` and CI both run
`.github/scripts/check-type-placement.sh`, which fails on a `type` declaration
anywhere else under `minio/`.

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

- Never edit files in `docs/` directly (edit `templates/` and run `task generate-docs`). The `docs/agents/` subdirectory is hand-written and not generated.
- Never bypass `NewResourceError()` in resource/data source CRUD.
- Never print with `fmt.Println` / `println`.
- Never ignore errors from `d.Set` or API calls.
- Never hardcode test resource names (always randomize).
- Never leave a comment that restates the code below it, or a commented-out code block.

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
| `task lint`          | Run configured linters                                 |

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

- **Formatting:** `gofmt -s` (CI enforces it through golangci-lint, whose `gofmt` formatter simplifies by default)
- **Linting:** Configured via `.github/golangci.yml` with errcheck, govet, ineffassign, staticcheck, unused, bodyclose, noctx, unconvert, and the gofmt formatter
- **Imports:** Group standard library, third-party, then internal
- **Naming:** Resources `resourceMinio<Name>()` in `resource_minio_<name>.go`, data sources `dataSourceMinio<Name>()` in `data_source_minio_<name>.go`, tests `TestAcc<Resource>_<scenario>`, payload structs `S3Minio<Name>`, extractors `<Name>Config`. Private functions camelCase, constants UPPER_SNAKE_CASE
- **Types:** Use explicit types for all function parameters and return values
- **Error handling:** Always `NewResourceError()` from `minio/error.go`

## Comment Hygiene

Comments are the exception, not the rule. A comment must explain *why*, never *what*.

- **Delete:** comments that restate the code, narration of self-evident steps, labels for entries already grouped by their own names, commented-out code blocks (history lives in git).
- **Keep:** non-obvious *why* (MinIO server or SDK workarounds, eventual-consistency handling, compatibility traps, state-convergence and security decisions), contracts and edge cases not visible in the signature, references to external docs or issues (e.g. `See issue #608`).
- When you change the code a comment describes, update or delete the comment in the same change.

## Testing

- Terraform Plugin SDK acceptance tests run against MinIO containers from `docker-compose.yml`. `go test` skips them unless `TF_ACC` is set; the Docker Compose runner is preferred.
- Randomize every test resource name (`acctest.RandString`). Minimum per resource: `_basic` and `_update`; an import test is recommended.
- An expectation built by the code under test is not an assertion. A check that computes its expected value with the same function that produced the actual value only proves the code agrees with itself. Assert against explicit literals, explicit lists of the actions or fields that matter, or a classification from an independent library (e.g. `policy.GetPolicy` from minio-go, which is what `mc` uses).
- The full testing rules, the instance list, the environment variables, and the common issues are in `docs/agents/testing-environment.md`.

## Detailed Guides

| Guide | Contents |
| ----- | -------- |
| [AI Coding Standards](docs/agents/ai-coding-standards.md) | Error handling, debug logging, payload pattern, CRUD structure, schema definitions, validation, type conversions, MinIO SDK notes, idempotency, performance, naming, provider registration, new-resource and new-data-source checklists |
| [Testing & Development Environment](docs/agents/testing-environment.md) | Test framework and rules, the four MinIO test instances, environment variables, running and skipping tests, common issues |
| [Documentation Workflow & Pull Request Guidelines](docs/agents/documentation-workflow.md) | Template structure and rules, `task generate-docs`, commit message format, pull request expectations |

## Reference Implementation & Questions

See `minio/resource_minio_s3_bucket_object_lock_configuration.go` as the reference implementation that follows ALL conventions in this document. If unclear about any convention:

1. Check existing resources for patterns
2. Refer to `resource_minio_s3_bucket_object_lock_configuration.go` (newest, follows all conventions)
3. Check `resource_minio_s3_object.go` for mature patterns
4. Look at recent commits in git history
