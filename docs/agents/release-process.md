# Release Process

How to cut a release of the provider. The release is automated by `.github/workflows/create-release-tag.yml`; the human side is one dispatch, one pull request review, and a few checks afterwards.

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

The `prepare` job then runs `.github/scripts/promote_changelog.py`, which moves the `[Unreleased]` entries into a `[<version>] - YYYY-MM-DD` section and updates the compare links. With entries to move, the job pushes branch `release/v<version>` and opens the release pull request as `github-actions[bot]`. Nothing is tagged yet.

A maintainer reviews that pull request and squash-merges it. Two things to check first:

- The diff touches `CHANGELOG.md` only.
- The promoted section holds exactly the entries that were under `[Unreleased]`.

The bot authors the pull request, so any maintainer's approval counts toward the required review. The squash commit subject `chore: release v<version>` is what triggers the rest: the push to `main` runs `tag-merged-release`, which creates the annotated tag (it refuses when `CHANGELOG.md` has no section for that version), and then the reusable `release.yml` builds and publishes through goreleaser: the GitHub release with the per-platform zips, `SHA256SUMS` and its signature, and the SBOM. The Terraform Registry picks up the tag on its own, a few minutes after the release goes public.

## After merging

```bash
gh release view v3.46.4 --json tagName,isDraft,isPrerelease,publishedAt
```

The release should not be a draft or a prerelease, and `publishedAt` should be set. For the registry, open <https://registry.terraform.io/publishers/aminueza/minio/versions> and check the version is listed. The whole chain, from dispatch to registry, took about 20 minutes for v3.46.4.

## CHANGELOG rules

- Every user-visible pull request adds its entry under `[Unreleased]` while it is open, not at release time. The format is [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and `Fixed` sits before `Security`.
- Concurrent pull requests all edit the top of the same section, so they conflict with each other as they merge. Resolve by merging `main` into the pull request branch and keeping both entries. Do not rebase: the branch belongs to a contributor's fork, and a rebase needs a force push.
