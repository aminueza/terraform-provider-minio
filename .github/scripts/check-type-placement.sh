#!/usr/bin/env bash
set -euo pipefail

STRAY=$(grep -rn "^type " minio/*.go | grep -v _test | grep -v payload.go || true)

if [ -n "$STRAY" ]; then
  MESSAGE="Type declarations belong in minio/payload.go. Move the declaration; its methods can stay where they are."
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    echo "::error::${MESSAGE}"
  else
    echo "Error: ${MESSAGE}" >&2
  fi
  echo "$STRAY"
  exit 1
fi

echo "Every type is declared in minio/payload.go."
