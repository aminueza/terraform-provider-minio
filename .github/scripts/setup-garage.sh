#!/usr/bin/env bash
# Configure a running Garage fixture so the acceptance suite can use it:
# wait for the RPC endpoint, apply the single-node cluster layout, then import
# the fixed S3 credentials that docker-compose.multi-backend.yml passes to the
# test service. Idempotent: the key import is the last step, so an existing
# key means the node is fully configured.
set -euo pipefail

cd "$(dirname "$0")/../.."

KEY_ID="GK000000000000000000000001"
SECRET_KEY="5ec3e7005ec3e7005ec3e7005ec3e7005ec3e7005ec3e7005ec3e7005ec3e700"
KEY_NAME="tfacc"

garage() {
	docker compose -f docker-compose.multi-backend.yml exec -T garage /garage "$@"
}

for attempt in $(seq 1 60); do
	if garage status >/dev/null 2>&1; then
		break
	fi
	if [ "$attempt" -eq 60 ]; then
		echo "Garage RPC endpoint did not become ready within 60s" >&2
		exit 1
	fi
	sleep 1
done

if garage key info "$KEY_ID" >/dev/null 2>&1; then
	echo "Garage is already configured (key $KEY_ID exists)"
	exit 0
fi

node_id="$(garage node id -q 2>/dev/null | head -1 | cut -d@ -f1)"
if ! garage layout show 2>/dev/null | grep -q "${node_id:0:16}"; then
	garage layout assign -z tfacc -c 10G "$node_id"
	garage layout apply --version 1
fi

garage key import --yes -n "$KEY_NAME" "$KEY_ID" "$SECRET_KEY"
garage key allow --create-bucket "$KEY_NAME"

echo "Garage is ready: S3 endpoint localhost:3900, key $KEY_ID"
