#!/usr/bin/env bash
# Prints the go test -skip regex for a backend from the support record in
# testdata/multi-backend/support.json: the OR of every resource pattern whose
# status is "unsupported" for that backend. Empty output means nothing to skip.
set -euo pipefail
cd "$(dirname "$0")/../.."

backend="${1:?usage: backend-skip.sh <backend>}"

jq -r --arg b "$backend" '
  .backends[$b] as $cfg
  | if $cfg == null then error("unknown backend: \($b)") else . end
  | .resources as $res
  | [ $cfg.support
      | to_entries[]
      | select(.value.status == "unsupported")
      | .key as $k
      | ($res[$k] // error("unsupported resource \($k) has no pattern in .resources"))
      | (if type == "array" then .[] else . end)
    ]
  | join("|")
' testdata/multi-backend/support.json
