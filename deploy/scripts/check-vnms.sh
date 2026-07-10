#!/usr/bin/env bash
# Test VNMS API reachability from the VSRV host (run on VSRV server).
set -euo pipefail

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$DEPLOY_DIR"

if [ -f .env ]; then
  # shellcheck disable=SC1091
  set -a && source .env && set +a
fi

: "${VNMS_BASE_URL:?set VNMS_BASE_URL in deploy/.env}"
: "${VNMS_HTTP_TOKEN:?set VNMS_HTTP_TOKEN in deploy/.env}"

echo "VNMS health:"
curl -fsS -H "Authorization: Bearer $VNMS_HTTP_TOKEN" "$VNMS_BASE_URL/healthz"
echo

echo "VNMS batch (empty):"
curl -fsS -H "Authorization: Bearer $VNMS_HTTP_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"device_ids":[]}' \
  "$VNMS_BASE_URL/v1/devices:batchGet"
echo
echo "VNMS API OK"
