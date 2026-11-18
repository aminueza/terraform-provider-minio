# Release Process

How to cut a release of the provider. The release is automated by `.github/workflows/create-release-tag.yml`; the human side is one dispatch, one pull request review, and a few checks afterwards, plus a one-time GitHub App setup described next.

## One-time setup: the release pull request App

`GITHUB_TOKEN` does not run CI on the pull requests it opens: before #1308 the release pull request was opened by `github-actions[bot]` and its four `pull_request` workflows either failed within seconds without running a job (#1301) or waited for someone to click "Approve workflows to run" (#1244). The workflow now opens it with a GitHub App token from `actions/create-github-app-token`, so the pull request has an author whose events do trigger workflows. There is no fallback to `GITHUB_TOKEN`: without the setup below, a release with changelog entries fails at the `Check the release pull request App secrets` step with an error naming the missing secret.

- Create a GitHub App at <https://github.com/settings/apps/new>. The name only shows up as the pull request author, for example `release-pr-bot`.
- Repository permissions: **Contents: Read and write** and **Pull requests: Read and write**, nothing else. The workflow asks the token for exactly these two (`permission-contents: write` and `permission-pull-requests: write`), and the action fails when it asks for a permission the installation does not have.
- Generate a private key on the app's page and copy the **Client ID** from its General page.
- Install the app on `aminueza/terraform-provider-minio`.
- Add both values as repository secrets under Settings → Secrets and variables → Actions: `RELEASE_PR_APP_CLIENT_ID` with the Client ID, `RELEASE_PR_APP_PRIVATE_KEY` with the private key.

The token is scoped to this repository, carries only those two permissions, and is revoked when the job ends.

## Before cutting a release

- The last push to `main` is green: `gh run list --branch main --event push`.
- `CHANGELOG.md` has the user-visible changes under `[Unreleased]`. A release with an empty `[Unreleased]` tags `main` without a pull request, so add the entries first.
- Cut releases when the maintainers can answer reports. Provider regressions show up over days, not minutes, so a dispatch right before a conference or a weekend is a bad time.

## Cutting the release

Start it with one command:

```bash
gh workflow run create-release-tag.yml --ref main -f version=3.46.4
```

Leave `version` empty and pass `-f release_type=patch|minor|major` instead to compute the next version from the latest tag.

The `prepare` job then runs `.github/scripts/promote_changelog.py`, which moves the `[Unreleased]` entries into a `[<version>] - YYYY-MM-DD` section and updates the compare links. With entries to move, the job pushes branch `release/v<version>` and opens the release pull request as the GitHub App's bot. Nothing is tagged yet.

A maintainer reviews that pull request and squash-merges it. Two things to check first:

- The diff touches `CHANGELOG.md` only.
- The promoted section holds exactly the entries that were under `[Unreleased]`.

With the App token in place, the checks look like they do on any other pull request: `Terraform Provider CI`, `Docs CI`, `Security Scan` and `Multi-Backend S3 Subset` start shortly after it opens and all four should be green before the merge. They no longer finish as failures seconds after being created without running a job, and nobody has to click "Approve workflows to run" (#1308).

The bot authors the pull request, so any maintainer's approval counts toward the required review. The squash commit subject `chore: release v<version>` is what triggers the rest: the push to `main` runs `tag-merged-release`, which creates the annotated tag (it refuses when `CHANGELOG.md` has no section for that version), and then the reusable `release.yml` builds and publishes through goreleaser: the GitHub release with the per-platform zips, `SHA256SUMS` and its signature, and the SBOM. The Terraform Registry picks up the tag on its own, a few minutes after the release goes public.

## After merging

```bash
gh release view v3.46.4 --json tagName,isDraft,isPrerelease,publishedAt
```

The release should not be a draft or a prerelease, and `publishedAt` should be set. For the registry, open <https://registry.terraform.io/publishers/aminueza/minio/versions> and check the version is listed. The whole chain, from dispatch to registry, took about 20 minutes for v3.46.4.

## CHANGELOG rules

- Every user-visible pull request adds its entry under `[Unreleased]` while it is open, not at release time. The format is [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and `Fixed` sits before `Security`.
- Concurrent pull requests all edit the top of the same section, so they conflict with each other as they merge. Resolve by merging `main` into the pull request branch and keeping both entries. Do not rebase: the branch belongs to a contributor's fork, and a rebase needs a force push.
