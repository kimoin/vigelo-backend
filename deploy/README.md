# VSRV deployment (UpCloud)

Deploys **vigelo-backend** (VSRV) on an UpCloud VM with Docker Compose: internal
PostgreSQL, the Go API, and Caddy for public HTTPS.

VNMS runs on a **separate VM** — see [`docs/upcloud-deploy.md`](../docs/upcloud-deploy.md)
for the full two-server setup and VNMS API integration.

## Architecture

```
Internet → Caddy :443 → vsrv :8090 → postgres (internal)
                ↓
         VNMS VM (private network)
         VNMS_BASE_URL + VNMS_HTTP_TOKEN
```

## Sync code to server

From your **local machine**:

```sh
SERVER=root@<vsrv-public-ip> ./deploy/scripts/sync-from-local.sh
```

Default destination: `/opt/vigelo-backend`

## First-time setup (on the VSRV server)

```sh
cd /opt/vigelo-backend/deploy

./scripts/01-install-docker.sh
# Log out/in or: newgrp docker

./scripts/02-firewall.sh
# Or restrict SSH: ADMIN_IP=<your.ip> ./scripts/02-firewall.sh

cp .env.example .env
./scripts/generate-secrets.sh   # paste values into .env

# Edit .env — required:
#   POSTGRES_PASSWORD, VSRV_HOSTNAME, VSRV_PUBLIC_URL
#   VNMS_BASE_URL (VNMS private IP), VNMS_HTTP_TOKEN (same as on VNMS server)
#   VSRV_ADMIN_EMAILS, FRONTEND_BASE_URL, VSRV_CORS_ORIGIN

./scripts/03-deploy.sh
```

Point DNS for `VSRV_HOSTNAME` to this server's public IP before first start
(Caddy needs it for Let's Encrypt).

## Verify

```sh
./scripts/smoke-test.sh          # public /healthz + VNMS /healthz
./scripts/check-vnms.sh          # VNMS API from this host
```

Admin console: `https://<VSRV_HOSTNAME>/admin/` (login with a user in `VSRV_ADMIN_EMAILS`).

Status tab should show **Database: ok** and **VNMS: ok** when integration works.

## VNMS integration checklist

On the **VNMS server** (`vigelo-nms/deploy`):

1. Set the same `VNMS_HTTP_TOKEN` in both `.env` files
2. Set `VNMS_HTTP_PUBLISH=<vnms-private-ip>:8080:8080`
3. Run `VSRV_PRIVATE_IP=<vsrv-private-ip> ./scripts/02-firewall.sh`
4. Import factory devices: `./scripts/import-inventory.sh devices.csv http://127.0.0.1:8080 $VNMS_HTTP_TOKEN`

On **VSRV**:

1. `VNMS_BASE_URL=http://<vnms-private-ip>:8080`
2. `VNMS_HTTP_TOKEN=<same token>`
3. Redeploy: `./scripts/03-deploy.sh`

## Device enrollment (end-to-end)

1. Factory-import device in VNMS (inventory → disabled)
2. User registers in app / VSRV
3. User claims device with `device_id` + enrollment secret → VSRV calls VNMS `verify-enrollment` + `enable`
4. Admin Status tab: VNMS **ok**, device appears under user in admin console

## Updates

```sh
# Local: sync + remote deploy
SERVER=root@<host> ./deploy/scripts/sync-from-local.sh
ssh root@<host> 'cd /opt/vigelo-backend/deploy && ./scripts/update.sh'
```

## Files

| File | Purpose |
|------|---------|
| `docker-compose.yml` | postgres, migrate, vsrv, caddy |
| `.env` | secrets (not committed) |
| `Caddyfile` | TLS reverse proxy |
| `certs/` | optional private CA for HTTPS to VNMS |
