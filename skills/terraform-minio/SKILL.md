---
name: terraform-minio
description: >-
  Author, plan, and safely apply Terraform for MinIO and S3-compatible object
  storage using the aminueza/minio provider. Use this skill whenever the user
  works with the MinIO Terraform provider, writes or reviews any minio_* resource
  or data source (minio_s3_bucket, minio_iam_user, minio_iam_policy,
  minio_iam_service_account, minio_ilm_policy, minio_s3_bucket_replication,
  minio_s3_bucket_versioning, ...), references the provider source aminueza/minio,
  or wants to create or change buckets, IAM users, groups, policies, service
  accounts, lifecycle rules, versioning, replication, encryption, or
  notifications in MinIO through Terraform. Also use it to translate
  natural-language requests like "give this app a read-only key for the backups
  bucket" into correct MinIO HCL, to debug minio provider plan/apply errors, to
  import existing MinIO resources into Terraform state, and to operate a MinIO
  endpoint with terraform (and the mc client) using a safe plan-then-confirm
  workflow.
---

# terraform-minio

An assistant for managing **MinIO** (and S3-compatible) object storage with the
[`aminueza/minio`](https://registry.terraform.io/providers/aminueza/minio/latest)
Terraform provider. It turns natural-language intent ("create a read-only user
for the `backups` bucket", "enable versioning + a 30-day expiry on `logs`") into
**correct HCL**, then drives Terraform through a **safe plan → confirm → apply**
loop — the same safe-by-default philosophy as `kubectl-ai`, adapted to Terraform.

The provider is large (65 resources, 54 data sources). This file covers the
operating model and the high-frequency surface. For the full inventory and exact
schemas, read the files under `references/` (pointers are given throughout).

## Operating principles

These exist because object-storage mistakes are expensive and often
irreversible (a wrong policy exposes a bucket publicly; `force_destroy` deletes
data; a leaked service-account key is a breach). Internalize the *why*, don't
just follow the steps.

1. **Act, don't narrate.** When `terraform` / `mc` are available, *run* the
   read-only and `plan` steps yourself and show the user the results — don't
   just print commands for them to copy. The user wants the outcome, not a
   tutorial. (Stop and hand control back before any mutating step — see the
   safety gate.)
2. **Safe by default.** Read-only operations run freely. Anything that mutates
   real infrastructure or state is **gated behind a reviewed plan and explicit
   user confirmation**. See [the safety gate](#the-safety-gate-read-vs-write).
3. **Plan before apply, always.** `terraform apply` without first showing the
   user a `terraform plan` is never acceptable. The plan *is* the
   confirmation summary. Use `-auto-approve` **only** after the user has seen a
   plan and said yes.
4. **Never invent values.** Don't guess bucket names, regions, ARNs, user names,
   or policy JSON. Inspect what exists first (read the `.tf` files,
   `terraform show`, `mc admin info`, the relevant data sources), then ask the
   user for anything still missing. Assumed defaults on object storage are how
   buckets end up public or data ends up in the wrong region.
5. **Secrets never go in HCL or state in plaintext.** Use variables + env vars
   for credentials; mark generated secrets sensitive; never echo a secret key.
   Prefer write-only secret arguments (`secret_wo`, Terraform ≥ 1.11) so secrets
   don't persist in state. See [Credentials & secrets](#credentials--secrets).
6. **Terraform owns managed resources — `mc` is for inspection.** Changing a
   managed resource with `mc` (or the console) causes drift and confusing
   diffs. Use `mc` to *look* (and to act on things Terraform doesn't manage),
   use Terraform to *change* what's in state.

## The working loop

1. **Understand intent.** Restate the goal in MinIO terms (which buckets,
   users, policies, permissions). Resolve ambiguity now, not after `apply`.
2. **Gather context.** Read existing `*.tf`, check the provider block and
   endpoint, and inspect live state where useful:
   - `terraform show` / `terraform state list` — what Terraform manages.
   - `mc admin info <alias>`, `mc ls <alias>`, `mc admin user list <alias>` —
     what actually exists on the server.
   - data sources (`minio_s3_buckets`, `minio_iam_users`, `minio_server_info`)
     when you want this inside the config.
3. **Author / modify HCL.** Use exact argument names (see the cheat-sheet and
   `references/resources.md`). Pin the provider. Keep secrets in variables.
4. **Validate + plan.** `terraform fmt`, `terraform validate`, then
   `terraform plan` (with `-input=false`). Read the diff yourself.
5. **Present the plan & confirm.** Summarize what will be created / changed /
   **destroyed** in plain language, call out anything dangerous (public access,
   `force_destroy`, deletions), and ask for explicit confirmation.
6. **Apply.** Only after confirmation. Then **verify** (`mc ls`, `mc stat`,
   re-run `plan` to show it's clean).

## The safety gate (read vs write)

Classify every command before running it. **Read-only → run freely. Mutating →
stop, show the plan/intent, get explicit confirmation.**

**Terraform — read-only (run freely):**
`init`, `validate`, `fmt`, `plan`, `show`, `output`, `state list`,
`state show`, `providers`, `providers schema`, `version`, `graph`.
(`init` only downloads providers/modules into the working dir — safe.)

**Terraform — mutating (require a reviewed plan + explicit confirmation):**
`apply`, `destroy`, `apply -destroy`, `import`, `state rm`, `state mv`,
`state push`, `state replace-provider`, `taint`, `untaint`,
`workspace delete`. Never pass `-auto-approve` unless the user approved a plan.

**mc — read-only (run freely):**
`mc ls`, `mc stat`, `mc du`, `mc tree`, `mc find`, `mc cat`, `mc alias list`,
`mc version`, `mc admin info`, `mc admin user list`, `mc admin group list`,
`mc admin policy list`/`info`, `mc admin config get`, `mc anonymous get`,
`mc ilm rule ls`, `mc replicate ls`.

**mc — mutating (require explicit confirmation):**
`mc mb`, `mc rb`, `mc rm`, `mc mv`, `mc cp`/`mc mirror` (to a target),
`mc anonymous set`, `mc admin user add`/`remove`/`enable`/`disable`,
`mc admin policy create`/`attach`/`detach`, `mc admin group add`/`rm`,
`mc admin config set`, `mc admin service restart`/`stop`,
`mc ilm rule add`/`rm`, `mc replicate add`/`rm`.

The single escape hatch is the user explicitly saying "go ahead / apply / skip
the confirmations". Honor it for that operation; don't make it the default.

## Provider setup essentials

The provider **source is `aminueza/minio`** (not `aminueza/terraform-provider-minio`).
`minio_server` is a bare `host:port` (no scheme); TLS is toggled by `minio_ssl`,
not by an `https://` prefix.

```hcl
terraform {
  required_providers {
    minio = {
      source  = "aminueza/minio"
      version = ">= 3.0.0"
    }
  }
}

provider "minio" {
  minio_server   = "localhost:9000"     # host:port, no scheme; env: MINIO_ENDPOINT
  minio_region   = "us-east-1"          # signing region (matters for S3-compat)
  minio_user     = var.minio_user       # access key;  env: MINIO_USER
  minio_password = var.minio_password   # secret key;  env: MINIO_PASSWORD (sensitive)
  minio_ssl      = false                # env: MINIO_ENABLE_HTTPS
}
```

- Canonical credential args are `minio_user` / `minio_password`. The older
  `minio_access_key` / `minio_secret_key` are **deprecated** and *conflict* if
  both pairs are set.
- Common extras: `minio_session_token`, `minio_insecure`, `minio_cacert_file`,
  `minio_api_version` (`v4`/`v2`), `s3_compat_mode` (for non-MinIO S3 backends
  like R2 / B2 / Spaces / Hetzner), plus `assume_role` and
  `assume_role_with_web_identity` blocks for STS / OIDC.
- Every provider arg has a `MINIO_*` env var. **Full reference, env-var table,
  STS/OIDC/mTLS, and S3-compat caveats: read `references/auth-and-config.md`.**

### Credentials & secrets

- Put credentials in **variables fed by env vars** (`TF_VAR_minio_user`, …) or
  the provider's own `MINIO_*` env vars — never hardcode them in committed HCL.
- Resources that **generate** secrets (`minio_iam_service_account`,
  `minio_accesskey`, `minio_iam_user` with a server-generated secret) expose
  `secret_key` as a **sensitive** computed attribute that lands in state. Treat
  state as secret, and when the provider/Terraform supports it use the
  **write-only** `secret_wo` argument (Terraform ≥ 1.11) so the value never
  persists in state.
- Never print a secret value back to the user in plaintext; reference where it
  lives (an output, the service account resource) instead.

## High-frequency resource cheat-sheet

Exact argument names for the resources people use most. **Get these right** —
wrong arg names are the #1 failure. Full schemas for these and every other
resource: `references/resources.md`. Copy-pasteable end-to-end recipes:
`references/recipes.md`.

| Resource | Key arguments (exact) | Notes / gotchas |
|---|---|---|
| `minio_s3_bucket` | `bucket` (or `bucket_prefix`), `acl`, `force_destroy`, `object_locking` | `acl` defaults to private; `force_destroy=true` deletes a non-empty bucket. |
| `minio_s3_bucket_policy` | `bucket`, `policy` (JSON) | Full custom bucket policy. |
| `minio_s3_bucket_versioning` | `bucket`, `versioning_configuration { status }` | `status = "Enabled"`. |
| `minio_s3_object` | `bucket_name`, `object_name`, `content`/`content_base64`/`source` | **Not** `bucket`/`key`. |
| `minio_iam_user` | `name`, `force_destroy`, `disable_user`, `secret`/`secret_wo`, `tags` | `secret_wo` (write-only) keeps the secret out of state (TF ≥ 1.11). |
| `minio_iam_group` | `name`, `force_destroy`, `disable_group` | |
| `minio_iam_group_membership` | `name`, `group`, `users` (set) | Authoritative membership for a group. |
| `minio_iam_policy` | `name` (or `name_prefix`), `policy` (JSON) | Managed policy; reference its `.id`. |
| `minio_iam_user_policy_attachment` | `user_name`, `policy_name` | **Not** `user`/`policy`. Import id: `user-name/policy-name`. |
| `minio_iam_group_policy_attachment` | `group_name`, `policy_name` | Attach a managed policy to a group. |
| `minio_iam_service_account` | `target_user` | Exports **sensitive** `access_key` / `secret_key`. App credentials. |

> Easily-confused pairs (which to use when) are spelled out in
> `references/resources.md` → *Clarifications*: the three group-membership
> resources, `s3_bucket_lifecycle` vs `ilm_policy`,
> `s3_bucket_anonymous_access` vs `s3_bucket_policy`, `service_account` vs
> `accesskey`, inline vs managed policy.

## The IAM permissions pattern

Don't hand-write policy JSON when you can build it with the
**`minio_iam_policy_document`** data source (AWS-IAM-style) and feed its `.json`
into a `minio_iam_policy`, then attach it. This is the canonical, least-error
path for "user/app X can do Y on bucket Z":

```hcl
data "minio_iam_policy_document" "backups_ro" {
  statement {
    sid       = "ReadBackups"
    actions   = ["s3:GetObject", "s3:ListBucket"]
    resources = ["arn:aws:s3:::backups", "arn:aws:s3:::backups/*"]
  }
}

resource "minio_iam_policy" "backups_ro" {
  name   = "backups-read-only"
  policy = data.minio_iam_policy_document.backups_ro.json
}

resource "minio_iam_user" "app" {
  name = "backups-reader"
}

resource "minio_iam_user_policy_attachment" "app_backups_ro" {
  user_name   = minio_iam_user.app.id   # note: user_name / policy_name
  policy_name = minio_iam_policy.backups_ro.id
}
```

Note the bucket needs **two** resource ARNs: the bucket itself (for
`ListBucket`) and `bucket/*` (for object actions). For machine credentials an
app actually authenticates with, prefer a `minio_iam_service_account` (it emits
an access/secret key pair) over embedding the user's own secret.

## When a resource isn't covered here

The provider evolves and is bigger than this file. To get an exact, current
schema rather than guessing:

- **Authoritative & offline:** `terraform providers schema -json | jq '.provider_schemas[].resource_schemas["minio_<name>"]'` after `terraform init`.
- **Docs:** `https://registry.terraform.io/providers/aminueza/minio/latest/docs/resources/<name>` (and `.../data-sources/<name>`). The URL uses the name **without** the `minio_` prefix.
- **Full inventory in this skill:** `references/resources.md` and
  `references/data-sources.md`.

## Reference map

Read the file that matches the task — don't load everything up front.

- `references/resources.md` — every resource grouped by area, one-line purpose
  each, exact schemas for the common ones, and the *Clarifications* section for
  confusing pairs. **Read when** writing/reviewing any resource beyond the
  cheat-sheet, or when unsure of an argument name.
- `references/data-sources.md` — every data source (singular/plural pairs,
  cluster-info sources). **Read when** querying existing MinIO state from HCL or
  building policy documents.
- `references/recipes.md` — copy-pasteable end-to-end configs (private bucket +
  versioning + lifecycle, public/anonymous bucket, read-only user, app service
  account, group + policy, replication, OIDC/LDAP attach, **importing existing
  resources**). **Read when** the user wants a complete working setup.
- `references/auth-and-config.md` — full provider configuration: every argument
  + env var, `assume_role`, web-identity/OIDC, mTLS, `s3_compat_mode`,
  timeouts/retries, edition. **Read when** setting up the provider, using STS,
  or targeting a non-MinIO S3 backend.

## Troubleshooting quick hits

- **`Error: Provider configuration ... AccessDenied` / signature errors** →
  wrong `minio_region` (S3-compat backends reject mismatched signing regions),
  or wrong credentials, or `minio_ssl` mismatch with the endpoint scheme.
- **`409` / "bucket already exists" on create** → the bucket exists out-of-band;
  `terraform import` it instead of recreating. Every resource is importable —
  per-resource import syntax is in `references/resources.md`.
- **Plan shows a forced replacement of a user/key you didn't change** → likely a
  `secret`/`update_secret` interaction; see the `secret` vs `secret_wo`
  clarification in `references/resources.md`.
- **Feature silently does nothing on a non-MinIO backend** → with
  `s3_compat_mode = true` the provider skips unsupported features
  (notifications, object-lock, CORS, lifecycle) on backends like R2/B2/Spaces.
- **Drift / unexpected diffs** → something changed the resource via `mc` or the
  console. Reconcile with `terraform apply` or `terraform import`, and stop
  managing it both ways.
