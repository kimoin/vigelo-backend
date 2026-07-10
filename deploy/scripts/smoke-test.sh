#!/usr/bin/env bash
# Smoke-test VSRV after deploy. Requires deploy/.env with VSRV_PUBLIC_URL set.
set -euo pipefail

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$DEPLOY_DIR"

if [ -f .env ]; then
  # shellcheck disable=SC1091
  set -a && source .env && set +a
fi

BASE="${VSRV_PUBLIC_URL:-https://${VSRV_HOSTNAME:-localhost}}"
echo "Checking $BASE/healthz"
curl -fsS "$BASE/healthz" | head -c 200
echo

if [ -n "${VNMS_BASE_URL:-}" ] && [ -n "${VNMS_HTTP_TOKEN:-}" ]; then
  echo "Checking VNMS at $VNMS_BASE_URL/healthz"
  curl -fsS -H "Authorization: Bearer $VNMS_HTTP_TOKEN" "$VNMS_BASE_URL/healthz"
  echo
else
  echo "Skip VNMS check (VNMS_BASE_URL or VNMS_HTTP_TOKEN not set in .env)"
fi

echo "OK"
