# Plan: Fix CI failures on PR #895

## Context

CI run [24561174354](https://github.com/aminueza/terraform-provider-minio/actions/runs/24561174354/job/71810131698)
on commit `04092a7` failed with 5 test failures after sections 0–8 of the
previous plan were executed. The provider now compiles and most tests pass,
but five specific regressions need to be fixed before PR #895 can merge.

```
--- FAIL: TestAccS3BucketReplication_oneway_simple
--- FAIL: TestAccMinioAccessKey_validation_requiresVersionWithWriteOnlySecret
--- FAIL: TestAccAWSUser_WriteOnlySecret_basic
--- FAIL: TestAccAWSUser_RotateAccessKey
--- FAIL: TestAccMinioAccessKey_validation_requiresWriteOnlySecretOnVersionChange
```

Three root causes underlie all five:

1. **Replication test configs use HCL block syntax** (`rule { ... }`) but the
   framework schema uses `ListNestedAttribute` (which requires `rule = [{ ... }]`).
2. **Write-only validators were over-removed** during the merge of PRs #899 /
   #900 — the merge dropped the check entirely instead of guarding it against
   unknown values.
3. **IAM user `secret` attribute leaks the write-only value** into state and
   rotation produces an inconsistent secret value on apply.

## Work order

1. [Fix `minio_s3_bucket_replication` block-vs-attribute mismatch](#1-replication-hcl-block-vs-attribute)
2. [Restore guarded validators on `minio_accesskey`](#2-accesskey-validators)
3. [Fix `minio_iam_user` write-only secret leak](#3-iam-user-write-only-secret-leak)
4. [Fix `TestAccAWSUser_RotateAccessKey` inconsistent sensitive value](#4-iam-user-rotate-access-key)
5. [Verify full CI run](#5-verify-full-ci-run)

Each section is a separate commit. Do them in order; 3 and 4 share code.

## Ground rules

- **Preserve the public HCL surface.** Existing user configs with `rule {}`
  blocks must continue to work — the migration from SDK to framework cannot be
  a breaking change on the `.tf` side.
- Run the specific failing test locally before pushing:
  `docker compose run --rm test go test -v -tags=acc -run '^<TestName>$' ./minio/...`
- Run `go build ./... && go vet ./...` after each commit.
- Do not widen scope. If you find a different bug, flag it.

---

## 1. Replication HCL block vs attribute

**Failure:**
```
on terraform_plugin_test.tf line 155, in resource "minio_s3_bucket_replication" "replication_in_b":
 155:   rule {
Blocks of type "rule" are not expected here. Did you mean to define argument
"rule"? If so, use the equals sign to assign it a value.
```

**Files:**
- [minio/resource_minio_s3_bucket_replication_framework.go:147](minio/resource_minio_s3_bucket_replication_framework.go:147) — `"rule": schema.ListNestedAttribute{...}`
- [minio/resource_minio_s3_bucket_replication_framework.go:200](minio/resource_minio_s3_bucket_replication_framework.go:200) — `"target": schema.ListNestedAttribute{...}`
- [minio/resource_minio_s3_bucket_replication_test.go](minio/resource_minio_s3_bucket_replication_test.go) — every test fixture uses `rule { ... }` block syntax (lines 23, 43, 63, 120, 140, 159, 293, 320, 346, 376, 575, 641, ...).

### Root cause

Plugin Framework distinguishes **attributes** (HCL `name = value`) from
**blocks** (HCL `name { ... }`). The migration used `ListNestedAttribute`,
which forces the HCL syntax `rule = [{ ... }]`. Existing user configs — and
all the test fixtures — write `rule { ... }` as a block. Terraform parses
that as a block type and the framework rejects it.

### Fix approach

Convert the schema to use **blocks**, not attributes, for `rule` and `target`.
This keeps existing HCL working. In the framework:

```go
resp.Schema = schema.Schema{
    Description: "...",
    Attributes: map[string]schema.Attribute{
        // non-block attributes here: bucket, etc.
    },
    Blocks: map[string]schema.Block{
        "rule": schema.ListNestedBlock{
            NestedObject: schema.NestedBlockObject{
                Attributes: map[string]schema.Attribute{
                    "id":       ...,
                    "enabled":  ...,
                    // ...
                },
                Blocks: map[string]schema.Block{
                    "target": schema.ListNestedBlock{
                        NestedObject: schema.NestedBlockObject{
                            Attributes: map[string]schema.Attribute{...},
                        },
                    },
                },
            },
        },
    },
}
```

### Steps

1. In [resource_minio_s3_bucket_replication_framework.go](minio/resource_minio_s3_bucket_replication_framework.go):
   - Move `"rule"` out of `Attributes` and into a new top-level `Blocks` map
     as `schema.ListNestedBlock`.
   - Move `"target"` out of the rule's nested `Attributes` into the rule's
     `Blocks` map as `schema.ListNestedBlock`.
   - Drop any `Required` / `Optional` / `Computed` markers on the block
     itself (blocks don't take those — validate count via `listvalidator.SizeAtLeast(1)`
     if needed).
   - Drop `planmodifier.List{...}` on `"rule"` — list-plan-modifiers bind to
     attributes, not blocks. The `secret_key` preservation must move to a
     **resource-level `ModifyPlan`** that walks `plan.rule` / `state.rule` by
     rule ID and writes back preserved values. Pattern:
     ```go
     func (r *bucketReplicationResource) ModifyPlan(
         ctx context.Context,
         req resource.ModifyPlanRequest,
         resp *resource.ModifyPlanResponse,
     ) {
         // read plan + state, walk rule→target, copy secret_key where needed,
         // resp.Plan.Set(ctx, &plan).
     }
     ```
2. Do the **same conversion** for the bucket-replication data source at
   [data_source_minio_s3_bucket_replication.go](minio/data_source_minio_s3_bucket_replication.go),
   so the data-source schema and resource schema stay aligned.
3. Do the **same conversion** for `minio_site_replication`'s `sites` if it is
   also a `ListNestedAttribute` — check
   [resource_minio_site_replication_framework.go](minio/resource_minio_site_replication_framework.go).
   Whether users wrote `sites { ... }` or `sites = [{ ... }]` in v2 determines
   the right call. If v2 used `sites` blocks (SDK `TypeList` + `Elem: &schema.Resource`),
   convert to `ListNestedBlock`. Grep existing user docs / examples to confirm.
4. Verify no other framework resources on this branch used `ListNestedAttribute`
   where the SDK equivalent was a block:
   ```bash
   grep -n "ListNestedAttribute" minio/*_framework.go
   ```
   Cross-reference each hit against its SDK predecessor's schema definition in
   git history (`git log --all -- <sdk-file>`). Anything that was a `TypeList`
   with `Elem: &schema.Resource{...}` in SDK v2 is a block in HCL and must
   become `ListNestedBlock` in the framework.

### Verification

```bash
docker compose run --rm test go test -v -tags=acc \
  -run '^TestAcc(S3BucketReplication|DataSourceMinioS3BucketReplication|MinioSiteReplication)' \
  ./minio/...
```

---

## 2. Accesskey validators

**Failures:**
- `TestAccMinioAccessKey_validation_requiresVersionWithWriteOnlySecret`
- `TestAccMinioAccessKey_validation_requiresWriteOnlySecretOnVersionChange`

Both report "expected an error with pattern, no match on: After applying this
test step, the plan was not empty." The tests expect the provider to raise a
validation error; the provider now accepts the config silently.

**File:** [minio/resource_minio_accesskey_framework.go](minio/resource_minio_accesskey_framework.go)

### Root cause

Merge commit `7fed215` (porting PR [#899](https://github.com/aminueza/terraform-provider-minio/pull/899))
deleted both validators entirely:

```diff
- 	// Static validation: if secret_key_wo is set, secret_key_wo_version must also be set.
- 		if plan.SecretKeyWOVersion.IsNull() || plan.SecretKeyWOVersion.IsUnknown() {
-               AddAttributeError(...)
+ 	// Static validation removed to allow unknown values during plan (matching SDK fix 244180b).
```

PR #899 does **not** remove the validation. It only suppresses the error
**when the value is `Unknown`** (i.e., sourced from an ephemeral resource
during plan). When both values are `Known`, the validation must still fire.

### Fix approach

Re-add both validators, guarded by `IsUnknown()`:

```go
// In ValidateConfig / ModifyPlan:
if !plan.SecretKeyWO.IsNull() && !plan.SecretKeyWO.IsUnknown() {
    if plan.SecretKeyWOVersion.IsNull() {
        resp.Diagnostics.AddAttributeError(
            path.Root("secret_key_wo_version"),
            "Missing secret_key_wo_version",
            "secret_key_wo_version must be provided when secret_key_wo is set",
        )
    }
    // If SecretKeyWOVersion.IsUnknown() — skip, let apply resolve it.
}

// Version-change validation:
if !plan.SecretKeyWOVersion.IsNull() && !plan.SecretKeyWOVersion.IsUnknown() &&
    !plan.SecretKeyWOVersion.Equal(state.SecretKeyWOVersion) {
    if plan.SecretKeyWO.IsNull() && !plan.SecretKeyWO.IsUnknown() {
        resp.Diagnostics.AddAttributeError(
            path.Root("secret_key_wo"),
            "Missing secret_key_wo",
            "secret_key_wo must be provided when secret_key_wo_version changes",
        )
    }
}
```

The distinction that matters: `IsNull()` = user left the attribute out, error
expected. `IsUnknown()` = value comes from a deferred reference (ephemeral
write-only secret); skip the check, let apply handle it.

### Steps

1. Re-read the upstream fix: `git show 244180b -- minio/resource_minio_accesskey.go`.
   Port the **intent**, not the line count.
2. Restore both validators with `IsUnknown()` guards as shown above.
3. Keep the existing apply-time check in `Update` that enforces the
   requirement when the version actually changes — it's the safety net for
   the unknown-at-plan case.

### Verification

```bash
docker compose run --rm test go test -v -tags=acc \
  -run '^TestAccMinioAccessKey_validation_' ./minio/...
```

Also run the full `TestAccMinioAccessKey_*` suite — the rotation-related
tests (`secretRotation`, `writeOnlySecretTransition`, `update`) passed in the
failing run and must still pass.

---

## 3. IAM user write-only secret leak

**Failure:**
```
resource_minio_iam_user_test.go:267: Step 1/1 error: Check failed:
Check 2/3 error: minio_iam_user.testwo:
  Attribute 'secret' expected "", got "QHNz_kQ0NecY4wRGk5-p96FP1eGyZkAMX0X7O2lOwaFfVs0MlWDdSQ=="
```

**File:** [minio/resource_minio_iam_user_framework.go](minio/resource_minio_iam_user_framework.go)

### Root cause

When a user configures only `secret_wo` (write-only), the `secret` attribute
must remain null / empty in state. The resource is instead populating
`secret` with the value that was supplied via `secret_wo`.

### Fix approach

1. In `Create` / `Update`, after calling MinIO admin and before writing state:
   - If the plan's `SecretWO` was set (not null / not unknown), write
     `state.Secret = types.StringNull()`.
   - If the plan's `Secret` was set, leave it as-is.
2. In `Read`, never set `Secret` from the server response if state previously
   had `Secret = null` (i.e., the user is on the write-only path). MinIO's
   user info does not round-trip the secret, so this should not be an issue,
   but guard explicitly.
3. The test at [resource_minio_iam_user_test.go:267](minio/resource_minio_iam_user_test.go:267)
   is the canonical check: `TestCheckResourceAttr("secret", "")`. Run it
   until it passes.

### Verification

```bash
docker compose run --rm test go test -v -tags=acc \
  -run '^TestAccAWSUser_WriteOnlySecret_basic$' ./minio/...
```

---

## 4. IAM user RotateAccessKey

**Failure:**
```
resource_minio_iam_user_test.go:145: Step 2/2 error: Error running apply:
Error: Provider produced inconsistent result after apply
  When applying changes to minio_iam_user.test3, provider produced an
  unexpected new value: .secret: inconsistent values for sensitive attribute.
```

**File:** [minio/resource_minio_iam_user_framework.go](minio/resource_minio_iam_user_framework.go),
[minio/resource_minio_iam_user_test.go:145](minio/resource_minio_iam_user_test.go:145)

### Root cause

On secret rotation, the plan carries the user-supplied new `secret`. The
resource calls MinIO to update the user, then in the state it writes a
different value than what was in the plan — typically because:

- The server returned a newly-generated secret (not the one sent), OR
- Code path is accidentally re-calling `SetUser` with a different value, OR
- A plan modifier is copying stale state into the new-value slot.

### Fix approach

1. Inspect the `Update` method in
   [resource_minio_iam_user_framework.go](minio/resource_minio_iam_user_framework.go).
   Verify the flow for the `secret` field:
   - Plan.Secret (new) → send to MinIO via `AddUser` / `SetUserSecret`.
   - Confirm the call uses the plan value, not a regenerated one.
   - After success, set `state.Secret = plan.Secret` (the value the user asked
     for). Do not re-fetch and trust the server.
2. If the MinIO admin client returns a generated secret when the plan value
   was empty/auto, confirm the plan actually carried a value — compare
   `plan.Secret` and the call payload.
3. Verify `Sensitive: true` on `secret` and `secret_wo` — the framework's
   consistency check is stricter on sensitive attributes, and marking them
   non-sensitive can mask bugs but is wrong.

Read [TF framework docs on inconsistent-value errors](https://developer.hashicorp.com/terraform/plugin/framework/diagnostics#provider-produced-inconsistent-result-after-apply)
before changing code — the fix is usually either "stop mutating state after
apply" or "mark the attribute `Computed`" (don't choose the latter for
user-supplied secrets).

### Verification

```bash
docker compose run --rm test go test -v -tags=acc \
  -run '^TestAccAWSUser_RotateAccessKey$' ./minio/...
```

Also run `^TestAccAWSUser_` broadly — rotation shares code with Update /
SettingAccessKey / UpdateAccessKey which currently pass.

---

## 5. Verify full CI run

After 1–4 land:

1. Local full-suite run:
   ```bash
   docker compose run --rm test
   ```
2. Push and watch the CI run. Expected green without any `TEST_SKIP`.
3. If CI is green and `gh pr checks` passes, mark PR [#895](https://github.com/aminueza/terraform-provider-minio/pull/895)
   ready for review and request maintainer merge.

## Out of scope

- Migrating remaining SDK data sources. Mux stays; follow-up PR.
- `madmin-go` upstream bugs.
- New features. This PR is migration-only.
