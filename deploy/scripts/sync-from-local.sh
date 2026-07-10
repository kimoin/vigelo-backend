#!/usr/bin/env bash
# Sync vigelo-backend to the VSRV server (no git credentials on server).
# Usage: SERVER=root@vsrv-host ./deploy/scripts/sync-from-local.sh
set -euo pipefail

SERVER="${SERVER:?set SERVER=user@host}"
DEST="${DEST:-/opt/vigelo-backend}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

ssh "$SERVER" "sudo mkdir -p '$DEST' && sudo chown \$(id -u):\$(id -g) '$DEST'"

rsync -az --delete \
  --exclude '.git' \
  --exclude 'deploy/.env' \
  --exclude '.env' \
  --exclude 'bin/' \
  "$REPO_ROOT/" "$SERVER:$DEST/"

echo "Synced to $SERVER:$DEST"
echo "Next: ssh $SERVER 'cd $DEST/deploy && ./scripts/03-deploy.sh'"
