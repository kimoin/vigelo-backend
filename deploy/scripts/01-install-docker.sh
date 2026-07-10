#!/usr/bin/env bash
# Install Docker Engine + Compose plugin on Ubuntu.
set -euo pipefail

if command -v docker >/dev/null 2>&1; then
  echo "docker already installed: $(docker --version)"
else
  curl -fsSL https://get.docker.com -o /tmp/get-docker.sh
  sudo sh /tmp/get-docker.sh
  rm -f /tmp/get-docker.sh
fi

if ! id -nG "$USER" | grep -qw docker; then
  sudo usermod -aG docker "$USER"
  echo "Added $USER to the docker group. Log out and back in (or run: newgrp docker)."
fi

sudo systemctl enable --now docker
docker --version
docker compose version
