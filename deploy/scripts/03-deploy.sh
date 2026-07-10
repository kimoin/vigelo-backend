#!/usr/bin/env bash
# Build and (re)start the VSRV stack.
set -euo pipefail

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$DEPLOY_DIR"

if [ ! -f .env ]; then
  echo "Missing deploy/.env. Copy deploy/.env.example and configure secrets."
  exit 1
fi

docker compose pull postgres caddy || true
docker compose build vsrv
docker compose up -d

docker compose ps
echo
echo "Logs: docker compose -f $DEPLOY_DIR/docker-compose.yml logs -f vsrv"
