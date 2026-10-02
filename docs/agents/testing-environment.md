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

# Build the MinIO servers from another release, for every MinIO service at once
MINIO_VERSION=RELEASE.2025-10-15T17-29-55Z docker compose run --rm test

# Run package tests
go test ./minio/...
```

The six MinIO services run an image built by `testdata/minio/Dockerfile`, which
compiles the `minio` server and the `mc` client from source through the Go
module proxy. On 2026-09-11 Docker Hub stopped serving `minio/minio` to
anonymous clients and every CI run went red at the pull step, before a single
test ran. The suite moved to `quay.io/minio/minio`, and on 2026-09-28 that
registry answered `unauthorized` too. GitHub Actions runners are anonymous
clients, so no registry MinIO controls is a safe source. The Go module proxy
serves the source of every tagged release and checks it against the checksum
database. `MINIO_VERSION` selects the server tag and `MC_VERSION` the client
tag; both apply to all six services. The image sets `MC_HOST_local` from the
root credentials of each service, so `mc ready local` and `mc admin info local`
work inside every container. In CI, `docker-compose.ci.yml` is merged in
through `COMPOSE_FILE` and stores the image layers in the GitHub Actions cache,
one scope per `MINIO_VERSION`, so a run with a warm cache skips the compile.
The override is not used locally, because the `gha` cache backend needs the
Actions runtime credentials.

## Hosted S3 Backends

`.github/workflows/hosted-backends.yml` runs the same S3 subset as the Garage
job against Cloudflare R2, Backblaze B2, DigitalOcean Spaces and Hetzner Object
Storage. It runs on `workflow_dispatch`, for one backend or all four, and every
Monday at 07:00 UTC. It never runs on `pull_request`, because a fork cannot read
the secrets and every outside contributor would see it fail.

Each backend reads four repository secrets. A backend with any of them missing
is skipped with a notice, not failed, so the workflow stays green until a
maintainer sets them.

| Backend | Secret prefix | Endpoint secret, host only | Region secret |
| --- | --- | --- | --- |
| Cloudflare R2 | `R2_` | `<account id>.r2.cloudflarestorage.com` | `auto` |
| Backblaze B2 | `B2_` | `s3.<region>.backblazeb2.com` | the region in the endpoint, e.g. `us-west-004` |
| DigitalOcean Spaces | `SPACES_` | `<region>.digitaloceanspaces.com` | the region in the endpoint, e.g. `nyc3` |
| Hetzner Object Storage | `HETZNER_` | `<location>.your-objectstorage.com` | the location, e.g. `fsn1` |

The four secrets are `<prefix>S3_ENDPOINT`, `<prefix>S3_REGION`,
`<prefix>S3_ACCESS_KEY` and `<prefix>S3_SECRET_KEY`. The region reaches the
provider through `MINIO_REGION`, because B2 and Hetzner reject a request signed
for another region.

Use an account, or a project inside one, that holds nothing but these runs.
After the tests, `tools/hosted-cleanup` removes every bucket whose name starts
with `tfacc` or `tf-` and that was created after the run started, emptying
noncurrent versions and delete markers first. It never touches a bucket created
before the run, but a key that can reach production buckets with those prefixes
should not be used here.

The record in `testdata/multi-backend/support.json` starts provisional for the
four services: only the groups that need the MinIO admin API, which no hosted
service serves, are marked `unsupported`. Everything else runs. The first run is
expected to fail; its job summary groups every test by its `support.json` group
with pass, fail and skip counts and the first error line of each failure. Copy
that into the record, a group per failure with the quoted error as the reason,
and the next run is green with the real map of what each service supports.

## Debugging a Failing Test

The steps are cumulative; each one adds to the ones above it.

1. Re-run with `-count=1`. `go test` caches a passing result, so a plain re-run
   of a test that just passed proves nothing.
2. Add `-v` for the per-step output of the test case.
3. Raise the log level with `TF_ACC_LOG=debug`. Do not use `TF_LOG` here: in an
   acceptance test the SDK reads `TF_LOG` as the Go standard library log level
   and sets Terraform's own `TF_LOG` from `TF_ACC_LOG`, so setting the former
   changes the framework's logging, not Terraform's. Use `TF_LOG_CORE` and
   `TF_LOG_PROVIDER` to separate Terraform core from provider logs.
4. Send the logs to a file with `TF_ACC_LOG_PATH=/tmp/tf-acc.log`, or
   `TF_LOG_PATH_MASK=/tmp/tf-acc-%s.log` for one file per test. `%s` is
   replaced with the test name. The test's Terraform working directory is
   always deleted on exit (`plugintest.WorkingDir.Close` calls `os.RemoveAll`)
   and SDKv2 has no switch to keep it, so the log file is the only record of
   the config and the plan. `TF_ACC_TEMP_DIR` only moves the directory, it does
   not preserve it.

A passing test can be a false negative. To prove a check asserts anything,
change the expected value in one of its `TestCheckFunc`s and run it again. A
test that still passes is not testing that field. Revert the edit once it fails
for the right reason.

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
