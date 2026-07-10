#!/usr/bin/env bash
# Generate random secrets for deploy/.env (print only — paste into .env manually).
set -euo pipefail

echo "# Paste into deploy/.env on VSRV and VNMS (VNMS_HTTP_TOKEN must match on both)"
echo "POSTGRES_PASSWORD=$(openssl rand -base64 24 | tr -d '/+=' | head -c 32)"
echo "VNMS_HTTP_TOKEN=$(openssl rand -hex 32)"
echo "VNMS_ADMIN_SESSION_KEY=$(openssl rand -hex 32)"
