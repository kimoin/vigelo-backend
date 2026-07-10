#!/usr/bin/env bash
# Firewall for the VSRV VM: SSH + public HTTPS API only.
# Restrict SSH: ADMIN_IP=1.2.3.4 ./02-firewall.sh
set -euo pipefail

ADMIN_IP="${ADMIN_IP:-}"

if ! command -v ufw >/dev/null 2>&1; then
  sudo apt-get update && sudo apt-get install -y ufw
fi

sudo ufw default deny incoming
sudo ufw default allow outgoing

if [ -n "$ADMIN_IP" ]; then
  echo "Restricting SSH to $ADMIN_IP"
  sudo ufw allow from "$ADMIN_IP" to any port 22 proto tcp
else
  echo "WARNING: allowing SSH from anywhere. Set ADMIN_IP to restrict."
  sudo ufw allow 22/tcp
fi

sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 443/udp

sudo ufw --force enable
sudo ufw status verbose
