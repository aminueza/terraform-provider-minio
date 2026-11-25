#!/usr/bin/env bash
# Validate every directory under examples/ with terraform init + validate.
#
# The provider is built from this checkout and served from a local filesystem
# mirror under both registry.terraform.io/aminueza/minio and
# registry.terraform.io/hashicorp/minio (snippets without required_providers
# resolve to hashicorp/minio). The mirror serves the current release version
# and the direct installer is excluded for both addresses, so a stale pin
# like version = "~> 2" fails at init instead of silently installing an old
# published provider. Providers the mirror does not hold (for example
# hashicorp/vault) are fetched from the registry.
#
# Usage: .github/scripts/validate-examples.sh [path-to-provider-binary]
set -euo pipefail

BINARY="${1:-terraform-provider-minio}"
if [ ! -f "$BINARY" ]; then
  echo "provider binary $BINARY not found; run go build -o $BINARY . first" >&2
  exit 1
fi
BINARY="$(cd "$(dirname "$BINARY")" && pwd)/$(basename "$BINARY")"
cd "$(dirname "$0")/../.."

VERSION=$(git ls-remote --tags --sort=-v:refname origin 'refs/tags/v*' | grep -E 'refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$' | sed -n 1p | sed 's|.*refs/tags/v||' || true)
if [ -z "$VERSION" ]; then
  echo "could not resolve the current release version from origin tags" >&2
  exit 1
fi
echo "serving provider v$VERSION from a local mirror"

PLATFORM="$(go env GOOS)_$(go env GOARCH)"
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT
MIRROR="$WORKDIR/mirror"
for ns in aminueza hashicorp; do
  dir="$MIRROR/registry.terraform.io/$ns/minio"
  mkdir -p "$dir/$VERSION/$PLATFORM"
  cp "$BINARY" "$dir/$VERSION/$PLATFORM/terraform-provider-minio_v$VERSION"
done

CLI_CONFIG="$WORKDIR/tfrc"
cat > "$CLI_CONFIG" <<EOF
provider_installation {
  filesystem_mirror {
    path    = "$MIRROR"
    include = ["registry.terraform.io/aminueza/minio", "registry.terraform.io/hashicorp/minio"]
  }
  direct {
    exclude = ["registry.terraform.io/aminueza/minio", "registry.terraform.io/hashicorp/minio"]
  }
}
EOF
export TF_CLI_CONFIG_FILE="$CLI_CONFIG"
export TF_IN_AUTOMATION=1

failed=0
count=0
while IFS= read -r -d '' dir; do
  ls "$dir"/*.tf >/dev/null 2>&1 || continue
  count=$((count + 1))
  # A lock file left by an earlier run carries no upstream checksums for the
  # mirrored provider, which makes every later init re-query the registry.
  # Start each directory from a clean slate so only vault hits the network.
  rm -f "$dir/.terraform.lock.hcl"
  echo "::group::$dir"
  if ! (cd "$dir" && terraform init -backend=false -input=false -no-color && terraform validate -no-color); then
    echo "::error::$dir failed terraform init or validate"
    failed=$((failed + 1))
  fi
  echo "::endgroup::"
done < <(find examples -type d -print0 | sort -z)

echo "validated $count directories, $failed failed"
[ "$failed" -eq 0 ]
