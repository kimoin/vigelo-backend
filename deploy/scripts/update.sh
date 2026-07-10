#!/usr/bin/env bash
# Pull latest code and redeploy on the server.
set -euo pipefail

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$DEPLOY_DIR"
./scripts/03-deploy.sh
