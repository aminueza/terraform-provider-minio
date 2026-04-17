# Plan: Finish PR #895 (SDK → Framework migration)

## Context

PR [aminueza/terraform-provider-minio#895](https://github.com/aminueza/terraform-provider-minio/pull/895)
is the SDK v2 → Plugin Framework migration. It has landed all resource
implementations on the framework, and the binary muxes the SDK server (data
sources only) with the framework server (resources) via
[main.go:35](main.go:35).

The previous developer closed out the secret_key blocker on
`minio_s3_bucket_replication` by **removing `secret_key` from the schema
entirely and adding `t.Skip(...)` to every replication acceptance test**. This
unblocked CI compile but did not actually resolve the migration: it silently
dropped a user-facing feature (`secret_key` credential management) and left
the tests unverified. We need to revert that shortcut and finish the job.

This plan enumerates what remains before PR #895 can be merged to `main`.

## Current state of skips

**CI-level (`.github/workflows/go.yml:94`):**

```
TEST_SKIP="TestAccS3BucketReplication|TestAccDataSourceMinioS3BucketReplication|TestAccMinioS3Bucket|TestAccMinioS3Object|TestAccMinioServerConfig|TestAccMinioSiteReplication|TestAccS3BucketVersioning"
```

**Source-level `t.Skip(...)` in tests not gated by env vars:**

| Test | File | Reason as written |
|---|---|---|
| `TestAccMinioS3Bucket_migrateBucketToBucketPrefix_incompatibleForcesReplacement` | [resource_minio_s3_bucket_test.go:58](minio/resource_minio_s3_bucket_test.go:58) | "conflicts with PLAN.md approach" |
| `TestAccMinioS3Bucket_Bucket_EmptyString` | [resource_minio_s3_bucket_test.go:194](minio/resource_minio_s3_bucket_test.go:194) | "conflicts with PLAN.md approach" |
| `TestAccMinioS3Bucket_migrateBucketToBucketPrefix` | [resource_minio_s3_bucket_test.go:273](minio/resource_minio_s3_bucket_test.go:273) | "conflicts with PLAN.md approach" |
| `TestAccMinioS3Bucket_migrateBucketToBucketPrefix_fromExactBucketName` | [resource_minio_s3_bucket_test.go:310](minio/resource_minio_s3_bucket_test.go:310) | "conflicts with PLAN.md approach" |
| `TestAccMinioS3Bucket_migrateBucketPrefixToBucket` | [resource_minio_s3_bucket_test.go:339](minio/resource_minio_s3_bucket_test.go:339) | "conflicts with PLAN.md approach" |
| `TestAccAWSUser_WriteOnlySecret_transition` | [resource_minio_iam_user_test.go:285](minio/resource_minio_iam_user_test.go:285) | "needs further investigation" |
| `TestAccS3BucketReplication_*` (7 tests) | [resource_minio_s3_bucket_replication_test.go](minio/resource_minio_s3_bucket_replication_test.go) | "secret_key limitation" |

(Skips gated by `MINIO_OIDC_ENABLED`, `MINIO_LDAP_ENABLED`, `MINIO_CORS_ENABLED`,
`MINIO_KMS_CONFIGURED`, or missing `MINIO_USER`/`MINIO_PASSWORD` are intentional
and stay.)

## Work order

Do these in order. Each section is a separate reviewable commit.

0. [Merge `origin/main` and resolve conflicts](#0-merge-main-and-resolve-conflicts)
1. [Restore `secret_key` on bucket replication (revert-and-fix)](#1-restore-secret_key-on-bucket-replication)
2. [Fix `secret_key` on site replication (same pattern)](#2-fix-secret_key-on-site-replication)
3. [Un-skip S3 bucket tests and CI](#3-un-skip-s3-bucket-tests-and-ci)
4. [Un-skip S3 bucket versioning in CI](#4-un-skip-s3-bucket-versioning-in-ci)
5. [Un-skip S3 object in CI](#5-un-skip-s3-object-in-ci)
6. [Un-skip server config in CI](#6-un-skip-server-config-in-ci)
7. [Finish IAM user write-only secret `_transition`](#7-finish-iam-user-write-only-secret-transition)
8. [Clear `TEST_SKIP` entirely and close out CI](#8-clear-test_skip-entirely)
9. [Decide scope: data sources on SDK vs framework](#9-decide-scope-data-sources)

## Ground rules

- **Never remove a `t.Skip(...)` or a `TEST_SKIP` entry without a local green
  run** of that exact test against the docker-compose MinIO instance.
- **Restore removed user-facing features.** `secret_key` was public API before
  PR #895 — dropping it is a breaking change and is not acceptable as the
  resolution path.
- **Preserve state of existing users.** A `terraform plan` against pre-migration
  state must be a no-op.
- **One resource per commit.** Do not bundle unrelated unskips.
- Run `go build ./... && go vet ./...` after every change.
- Acceptance tests: `docker compose run --rm test go test -v -tags=acc -run '^<pattern>$' ./minio/...`
  mirroring [.github/workflows/go.yml](.github/workflows/go.yml).

---

## 0. Merge `origin/main` and resolve conflicts

GitHub reports the PR has conflicts on two files:

- `minio/resource_minio_accesskey.go`
- `minio/resource_minio_iam_idp_openid.go`

Both files have been **deleted on this branch** and replaced by framework
implementations:

- [minio/resource_minio_accesskey_framework.go](minio/resource_minio_accesskey_framework.go)
- (openid is rolled into the framework IAM IDP stack; confirm at merge time)

Meanwhile `main` has landed two fixes against the old SDK files:

| Commit on `main` | File | What it fixes |
|---|---|---|
| [244180b](https://github.com/aminueza/terraform-provider-minio/commit/244180b) (PR [#899](https://github.com/aminueza/terraform-provider-minio/pull/899)) | `resource_minio_accesskey.go` | Allow unknown value for `secret_key_wo` during plan |
| [372e48c](https://github.com/aminueza/terraform-provider-minio/commit/372e48c) (PR [#900](https://github.com/aminueza/terraform-provider-minio/pull/900)) | `resource_minio_iam_idp_openid.go` | Allow unknown value for `client_secret_wo` during plan |

### Steps

1. On the feature branch:

   ```bash
   git fetch origin main
   git merge origin/main
   ```

2. For each conflicted file, resolve by **keeping the framework version** (i.e.
   the file stays deleted on this branch). Run:

   ```bash
   git rm minio/resource_minio_accesskey.go
   git rm minio/resource_minio_iam_idp_openid.go
   ```

3. **Port the fix forward to the framework implementation.** The SDK fix
   pattern is: during plan, if `*_wo_version` is unknown / changed, allow the
   `*_wo` value to be unknown rather than forcing it from state. Apply the
   equivalent logic in:

   - [minio/resource_minio_accesskey_framework.go](minio/resource_minio_accesskey_framework.go) —
     mirror [244180b](https://github.com/aminueza/terraform-provider-minio/commit/244180b)
     for `secret_key_wo`.
   - The framework IAM IDP OpenID resource — mirror
     [372e48c](https://github.com/aminueza/terraform-provider-minio/commit/372e48c)
     for `client_secret_wo`. Locate the framework file first
     (`grep -l OIDC minio/*_framework.go`) since the file layout may differ
     from the SDK version.

   Read both upstream commits in full (`git show 244180b` / `git show 372e48c`)
   before porting. Don't assume the SDK plan-modifier mechanism maps 1:1 to
   framework plan modifiers; the framework equivalent is usually setting the
   plan value to `types.StringUnknown()` under the same condition, or using a
   custom `planmodifier.String` that no-ops when version is unknown.

4. Verify:

   ```bash
   go build ./... && go vet ./...
   docker compose run --rm test \
     go test -v -tags=acc -run '^TestAccAccesskey|^TestAccMinioIamIDPOpenID' \
     ./minio/...
   ```

   The OpenID tests are env-gated (`MINIO_OIDC_ENABLED=1`) — run locally with
   the env var set, or rely on targeted manual verification.

5. Commit the merge as a single commit (do not squash the main history);
   include a short note in the commit body pointing to PRs #899 and #900 and
   explaining that their fixes were re-applied against the framework files.

### Gotchas

- Do **not** re-introduce the deleted SDK files just to apply the `main` fix
  and then delete them again. That churns history for no gain — port the fix
  directly to the framework file.
- If any other files conflict during the actual merge (beyond the two GitHub
  highlighted), apply the same rule: framework implementation wins, but
  re-read the `main`-side change to see if its intent needs porting.
- Re-run the full build + lint + the CI's current non-skipped tests after the
  merge to catch indirect breakage from anything else that landed on `main`
  since the branch diverged.

---

## 1. Restore `secret_key` on bucket replication

**Files:**
- [minio/resource_minio_s3_bucket_replication_framework.go:88](minio/resource_minio_s3_bucket_replication_framework.go:88) — schema comment that documents removal
- [minio/resource_minio_s3_bucket_replication_framework.go:664](minio/resource_minio_s3_bucket_replication_framework.go:664) — hard-coded empty `SecretKey: ""`
- [minio/resource_minio_s3_bucket_replication_test.go](minio/resource_minio_s3_bucket_replication_test.go) — 7 `t.Skip(...)` calls
- [minio/data_source_minio_s3_bucket_replication.go](minio/data_source_minio_s3_bucket_replication.go) — mirror of resource schema

**Context:** Commit [6063f51](https://github.com/aminueza/terraform-provider-minio/commit/6063f51)
removed `secret_key` from the schema because MinIO's API does not return it on
Read (it is write-only), which caused the framework's consistency check to
fail on every refresh. This is a real framework limitation, but the fix is not
"delete the attribute". The fix is to teach the resource to preserve state
for `secret_key`.

### Root cause

`target.secret_key` lives inside a `ListNestedAttribute`. Plan modifiers on
nested attributes inside list elements are not expressible directly; the
framework does not surface `UseStateForUnknown()` per-element. Without that,
Read returns `""` where state had the user's value, and Terraform errors.

### Fix approach — Option A (preferred)

1. Add `secret_key` back to `replicationTargetObjectType` (sensitive string).
2. Write a `planmodifier.List` on the `rule` attribute that:
   - Matches plan-rule ↔ state-rule by `id`.
   - For each matched rule, walks `target[*]` and, when plan's
     `target[i].secret_key` is null/empty, copies state's value into plan.
   - Leaves the value alone when the user set a new one (rotation).
3. In `applyReplication` / `readReplicationTarget`, read `secret_key` from the
   plan value passed in (not from the server — the server never returns it)
   and pass it through to `madmin.BucketTarget.Credentials.SecretKey`.
4. In Read, **never overwrite `secret_key` from the server response.** Read it
   from prior state and re-emit it.
5. Remove every `t.Skip(...)` added by commit
   [6063f51](https://github.com/aminueza/terraform-provider-minio/commit/6063f51).
6. Restore `secret_key` in every test fixture that commit stripped.

### Fix approach — Option B (fallback, only if A doesn't hold)

Split into `minio_s3_bucket_replication` (bucket-scoped) +
`minio_s3_bucket_replication_rule` (one per rule) so plan modifiers can live
at the top level. This is a breaking-change refactor; do not choose it unless
Option A is demonstrably impossible.

### Also fix while you're in this file

- `status` per-rule — add `stringvalidator.OneOf("Enabled", "Disabled")`.
- Priority uniqueness validator across rules (plan-time).

### Verification

```bash
docker compose run --rm test \
  go test -v -tags=acc \
  -run '^TestAcc(S3BucketReplication|DataSourceMinioS3BucketReplication)' \
  ./minio/...
```

All 7 suite tests must pass with `secret_key` set in HCL.

### Commit

- Remove `TestAccS3BucketReplication|TestAccDataSourceMinioS3BucketReplication`
  from the `TEST_SKIP` in [.github/workflows/go.yml:94](.github/workflows/go.yml:94).

---

## 2. Fix `secret_key` on site replication

**Files:**
- [minio/resource_minio_site_replication_framework.go](minio/resource_minio_site_replication_framework.go)
- [minio/resource_minio_site_replication_test.go](minio/resource_minio_site_replication_test.go)

**CI skip pattern:** `TestAccMinioSiteReplication`

### Root cause

Identical to section 1: `sites` is a list-nested attribute, each site carries
`secret_key` (and likely `secret_key_wo` + `secret_key_wo_version`) which the
server never returns. No per-element plan modifiers.

### Fix approach

Apply the same `planmodifier.List` pattern from section 1 to the `sites` list:

1. Pair plan-site ↔ state-site by `deployment_id` (or `endpoint` if
   `deployment_id` is unknown on create).
2. Copy `secret_key` from state into plan when plan's value is null.
3. For `secret_key_wo`: key the rotation on `secret_key_wo_version` — copy
   state into plan when version is unchanged, use plan's new value when
   version bumps (see section 7 for the version-aware pattern).
4. In Read, preserve the prior state values for these fields; never take them
   from the server.
5. Confirm `enable_ilm_expiry_replication` round-trips unchanged.

Once sections 1 and 2 are both done, consider extracting the shared modifier
into `minio/plan_modifiers.go` as `listPreserveSensitiveByKey(pairKey,
sensitiveFields...)`. **Only extract after the second use, not before.**

### Verification

```bash
docker compose run --rm test \
  go test -v -tags=acc -run '^TestAccMinioSiteReplication' ./minio/...
```

### Commit

- Remove `TestAccMinioSiteReplication` from `TEST_SKIP` in CI.

---

## 3. Un-skip S3 bucket tests and CI

**Files:**
- [minio/resource_minio_s3_bucket_test.go](minio/resource_minio_s3_bucket_test.go)
- [minio/resource_minio_s3_bucket_framework.go](minio/resource_minio_s3_bucket_framework.go)

**CI skip pattern:** `TestAccMinioS3Bucket`
**Source `t.Skip`s:** 4 migration/empty-string tests (lines 58, 194, 273, 310, 339).

### What to verify first

Commit [3b07a9a](https://github.com/aminueza/terraform-provider-minio/commit/3b07a9a)
("update S3 bucket resource framework and tests") claims completion. Run the
full `TestAccMinioS3Bucket` suite locally against docker-compose before
touching anything. If it passes clean, skip to "Remove the skip pattern".

### Re-enable the 4 migration tests

The skip messages say "conflicts with PLAN.md approach — bucket_prefix is
create-time only with RequiresReplace". That justification is backwards: the
tests exist precisely to assert the migration behavior between `bucket` and
`bucket_prefix`. Either:

- **The tests are correct** and the resource needs to handle the migration
  (likely via a state upgrader or careful plan modifier interaction on
  `bucket_prefix` ↔ `bucket`) — implement it and unskip.
- **The tests are obsolete** because the feature was intentionally dropped —
  delete them and note the breaking change in the PR description. Requires
  sign-off; do not decide unilaterally.

Default to the first interpretation; ask before choosing the second.

The `TestAccMinioS3Bucket_Bucket_EmptyString` case asserts that an empty
`bucket` string is valid (prefix-only). Resolve by validating inputs: either
`bucket` or `bucket_prefix` must be set but not both — enforce via
`resource.ConfigValidators` and adjust the test to match.

### Verification

```bash
docker compose run --rm test \
  go test -v -tags=acc -run '^TestAccMinioS3Bucket' ./minio/...
```

### Commit

- Remove each `t.Skip(...)` only after the corresponding test passes.
- Remove `TestAccMinioS3Bucket` from CI `TEST_SKIP`.

---

## 4. Un-skip S3 bucket versioning in CI

**Files:**
- [minio/resource_minio_s3_bucket_versioning_framework.go](minio/resource_minio_s3_bucket_versioning_framework.go)
- [minio/resource_minio_s3_bucket_versioning_test.go](minio/resource_minio_s3_bucket_versioning_test.go)

Commit [d95a04c](https://github.com/aminueza/terraform-provider-minio/commit/d95a04c)
claims completion. No source-level `t.Skip`, only the CI pattern
`TestAccS3BucketVersioning`.

### Steps

1. Run the full suite locally.
2. If any test fails, diagnose via `TF_LOG=DEBUG` looking for "unexpected new
   value" diffs on `exclude_folders`, `excluded_prefixes`, or
   `versioning_configuration.status`.
3. Fix per the usual patterns (explicit defaults, empty-list vs null-list
   handling, enum validators).
4. Remove `TestAccS3BucketVersioning` from CI `TEST_SKIP`.

### Verification

```bash
docker compose run --rm test \
  go test -v -tags=acc -run '^TestAccS3BucketVersioning' ./minio/...
```

---

## 5. Un-skip S3 object in CI

**Files:**
- [minio/resource_minio_s3_object_framework.go](minio/resource_minio_s3_object_framework.go)
- [minio/resource_minio_s3_object_test.go](minio/resource_minio_s3_object_test.go)

Commit [6a8ad0c](https://github.com/aminueza/terraform-provider-minio/commit/6a8ad0c)
claims completion. CI pattern: `TestAccMinioS3Object`. No source `t.Skip`.

### Steps

1. Run the suite locally. `ImportStateVerifyIgnore` at
   [resource_minio_s3_object_test.go:39](minio/resource_minio_s3_object_test.go:39)
   already lists `content`, `content_base64`, `source`, `acl` — verify that's
   sufficient.
2. Confirm `etag` / `version_id` round-trip via `UseStateForUnknown()`.
3. Remove `TestAccMinioS3Object` from CI `TEST_SKIP`.

### Verification

```bash
docker compose run --rm test \
  go test -v -tags=acc -run '^TestAccMinioS3Object' ./minio/...
```

---

## 6. Un-skip server config in CI

**Files:** all 6 of `minio/resource_minio_server_config_*_framework.go` and
[minio/resource_minio_server_config_test.go](minio/resource_minio_server_config_test.go).

Commit [51ee848](https://github.com/aminueza/terraform-provider-minio/commit/51ee848)
claims completion. CI pattern: `TestAccMinioServerConfig`. No source `t.Skip`.

### Steps

1. Run the full suite locally.
2. For each failing attribute, verify:
   - `restart_required` is `Computed` only, with `UseStateForUnknown()`.
   - Enum fields have `stringvalidator.OneOf(...)` plus matching defaults
     (e.g. `scanner.speed` ∈ `{slow, default, fast}`, `heal.bitrotscan` ∈
     `{on, off}`).
   - `Optional + Computed` attributes that mirror server defaults have an
     explicit `stringdefault`/`booldefault` matching the server.
3. Remove `TestAccMinioServerConfig` from CI `TEST_SKIP`.

### Verification

```bash
docker compose run --rm test \
  go test -v -tags=acc -run '^TestAccMinioServerConfig' ./minio/...
```

---

## 7. Finish IAM user write-only secret `_transition`

**File:** [minio/resource_minio_iam_user_test.go:285](minio/resource_minio_iam_user_test.go:285)

Commit [b1f9b86](https://github.com/aminueza/terraform-provider-minio/commit/b1f9b86)
landed the `_basic` case. The `_transition` case (moving from `secret` to
`secret_wo`) is still skipped with "needs further investigation".

### Fix approach

1. Verify `Update` recognizes the migration: when `secret_wo_version` is set
   on plan and `secret` was set on state, prefer `secret_wo`.
2. After Update, null the `secret` attribute in state (the user has moved to
   write-only).
3. Confirm `secret_wo` is marked `Sensitive: true` + `WriteOnly: true`.
4. The rotation modifier must key on `secret_wo_version`: unchanged version =
   plan value forced to null (no diff); changed version = new value flows to
   Update.

### Verification

```bash
docker compose run --rm test \
  go test -v -tags=acc -run '^TestAccAWSUser_WriteOnlySecret' ./minio/...
```

Run the rest of `TestAccAWSUser_*` to confirm no regression.

---

## 8. Clear `TEST_SKIP` entirely

After sections 1–7 land and each suite passes:

1. [.github/workflows/go.yml:94](.github/workflows/go.yml:94) should reduce to:

   ```yaml
         - name: Run tests
           run: docker compose run --rm test
   ```

2. Confirm a full green CI run on the branch.
3. Confirm no `t.Skip(...)` remains in source outside env-gated tests
   (LDAP / OIDC / CORS / KMS).

```bash
grep -rn "t\.Skip(" minio/
```

The only lines that should match are env-gated (checking
`MINIO_OIDC_ENABLED`, `MINIO_LDAP_ENABLED`, `MINIO_CORS_ENABLED`,
`MINIO_KMS_CONFIGURED`, `MINIO_USER`/`MINIO_PASSWORD`).

---

## 9. Decide scope: data sources

[minio/provider.go](minio/provider.go) still holds 33 data sources on SDK v2.
[minio/provider_framework.go:329](minio/provider_framework.go:329) only
registers one (`newS3BucketDataSource`). The binary muxes both servers via
[main.go:35](main.go:35), so users are unaffected today.

### Options

- **Merge PR #895 with data sources still on SDK.** Document in the PR that
  resource migration is complete and data-source migration will follow in a
  separate PR. The mux stays. This is the smaller, lower-risk path.
- **Migrate all data sources inside PR #895.** ~33 data sources × the full
  schema/CRUD cycle is substantial. Only do this if the maintainers insist on
  a single atomic migration.

**Recommendation:** ship sections 1–8 in PR #895 and open a follow-up PR for
the data-source sweep. Confirm this with the maintainers on the PR thread
before merging. If they want atomic, plan the data-source work as its own
section-per-data-source effort mirroring the resource migration cadence.

---

## Out of scope (flag separately if encountered)

- MinIO server / `madmin-go` upstream bugs. File issues upstream; do not patch
  locally.
- Resources gated by `MINIO_OIDC_ENABLED`, `MINIO_LDAP_ENABLED`,
  `MINIO_CORS_ENABLED`, `MINIO_KMS_CONFIGURED`. These skips are intentional.
- Documentation regeneration. Run `go generate ./...` as a final step once
  all schema changes land; treat any diff as a blocker to merge.
