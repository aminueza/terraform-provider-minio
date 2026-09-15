# Testing & Development Environment

How to run the provider test suites and what the local MinIO instances are.
The short version of the mandatory rules is in the repository root `AGENTS.md`.

## Test Framework

- **Framework:** Terraform Plugin SDK acceptance tests
- **Environment:** Tests run against MinIO containers defined in `docker-compose.yml`
- **Naming:** `TestAcc<ResourceName>_<scenario>` (e.g., `TestAccMinioS3Bucket_basic`)
- **Requirements:** Each resource must have acceptance tests covering create, read, update, delete, and import
- **Test structure:** Use `resource.Test` with `TestCase` containing steps
- **PreCheck:** Implement `testAccPreCheck(t)` for provider configuration validation
- **Expectations:** An expectation built by the code under test is not an assertion. A check that computes its expected value with the same function that produced the actual value only proves the code agrees with itself, so no change to that function can fail it. Assert against an explicit literal, an explicit list of the actions or fields that matter, or a classification from an independent library (e.g. `policy.GetPolicy` from minio-go, which is what `mc` uses). A golden comparison is fine for change detection, but pair it with an independent assertion instead of leaving it as the only check.

## Test Environment Instances

**Available MinIO instances:**

- Primary: `localhost:9000` (minio/minio123)
- Second: `localhost:9002` (minio/minio321)
- Third: `localhost:9004` (minio/minio456)
- Fourth: `localhost:9006` (minio/minio654)

**Environment variables:**

`testAccPreCheck` requires the 16 variables below for the four core instances (primary, SECOND_, THIRD_, FOURTH_). The `test` service in `docker-compose.yml` also defines `KMS_MINIO_*` and `LDAP_MINIO_*` variables — these are optional and only needed when running the KMS key or LDAP identity provider test suites. For local runs against the published ports:

```bash
export TF_ACC=1
export MINIO_ENDPOINT=localhost:9000
export MINIO_USER=minio
export MINIO_PASSWORD=minio123
export MINIO_ENABLE_HTTPS=false
export SECOND_MINIO_ENDPOINT=localhost:9002
export SECOND_MINIO_USER=minio
export SECOND_MINIO_PASSWORD=minio321
export SECOND_MINIO_ENABLE_HTTPS=false
export THIRD_MINIO_ENDPOINT=localhost:9004
export THIRD_MINIO_USER=minio
export THIRD_MINIO_PASSWORD=minio456
export THIRD_MINIO_ENABLE_HTTPS=false
export FOURTH_MINIO_ENDPOINT=localhost:9006
export FOURTH_MINIO_USER=minio
export FOURTH_MINIO_PASSWORD=minio654
export FOURTH_MINIO_ENABLE_HTTPS=false
```

With these set, single-instance tests (e.g. `TestAccMinioS3Bucket_basic`) only need the primary instance running: `docker compose up -d minio`. Multi-instance tests (replication, site replication) additionally need their instances up and reachable from the MinIO servers themselves, so they are best run via `docker compose run --rm test`.

## Running Tests

```bash
# Run all tests
docker compose run --rm test

# Run specific test
TEST_PATTERN=TestAccMinioIAMUser docker compose run --rm test

# Run with verbose output
TF_ACC=1 go test -v ./minio -run TestAccMinioS3Bucket_basic

# Take the MinIO images from another registry, for every MinIO service at once
MINIO_IMAGE=minio/minio:RELEASE.2025-09-07T16-13-09Z docker compose run --rm test

# Run package tests
go test ./minio/...
```

The default is `quay.io/minio/minio`. On 2026-09-11 Docker Hub stopped serving
`minio/minio` to anonymous clients and every CI run went red at the pull step,
with `pull access denied for minio/minio`, before a single test ran. A probe
from a runner found the pinned tag and `latest` on `quay.io` and on no other
public registry: `ghcr.io/minio/minio` and `public.ecr.aws/minio/minio` do not
carry it. `MINIO_IMAGE` overrides the image for all six MinIO services, so a
future move needs one variable rather than an edit to `docker-compose.yml`.

## Common Issues

**Config KV resources (notify_\*, audit_\*, logger_\*, server_config_\*):** These use `SetConfigKV`/`GetConfigKV`/`DelConfigKV` instead of the payload struct pattern. Named targets (e.g., `notify_kafka:primary`) use the shared helpers in `resource_minio_notify_common.go`. Singleton subsystems (e.g., `api`, `scanner`, `heal`) use the subsystem name as resource ID. Some subsystems (notably `notify_*` and `region`) require a server restart before `GetConfigKV` returns new values — handle the "there is no target" error by keeping state as-is.

**Credentials and import:** MinIO returns `REDACTED` for sensitive fields (secret keys, passwords, tokens). Always add credential fields to `ImportStateVerifyIgnore` in tests.

**ILM tier names:** Must be UPPERCASE:

```go
tierName := strings.ToUpper("TFACC-TIER-" + acctest.RandString(8))
```

**Object lock:** Requires versioning enabled. Needs MinIO RELEASE.2025-05-20+ to add after bucket creation.

**LDAP/KMS tests:** Skip automatically if not configured. Set `MINIO_LDAP_ENABLED=1` or `MINIO_KMS_CONFIGURED=1` to run.

**Skipping tests in Docker:** Use `TEST_SKIP` (not `TEST_PATTERN`) for the `-skip` flag: `docker compose run --rm -e TEST_SKIP=TestName test`. `TEST_PATTERN` only supports `-run` patterns.

**Template engine limitations:** `trimprefix` and other Go text/template functions are NOT available in tfplugindocs. Hardcode values instead.
