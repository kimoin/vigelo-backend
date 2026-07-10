# UpCloud deployment — VSRV + VNMS

This guide connects **two UpCloud VMs** so VSRV can use the VNMS HTTP API for
device enrollment, enable/disable, schedule sync, and event polling.

## Topology

| VM | Service | Public ports | Private |
|----|---------|--------------|---------|
| **vm-vnms** | vigelo-nms | UDP 5642 (devices) | HTTP 8080 → VSRV only |
| **vm-vsrv** | vigelo-backend | HTTPS 443 (API + admin) | — |

Both VMs should be on the same **UpCloud private network** (SDN). Note each
VM's private IP (e.g. VNMS `10.0.1.10`, VSRV `10.0.1.20`).

## 1. Generate shared secrets (local)

```sh
cd vigelo-backend/deploy
./scripts/generate-secrets.sh
```

Save output — use the same `VNMS_HTTP_TOKEN` on **both** servers.

## 2. Deploy VNMS (vm-vnms)

```sh
# From local machine
cd vigelo-nms
SERVER=root@<vnms-public-ip> ./deploy/scripts/sync-from-local.sh

# On VNMS server
cd /opt/vigelo-nms/deploy
./scripts/01-install-docker.sh
cp .env.example .env
# Edit .env:
#   POSTGRES_PASSWORD
#   VNMS_HTTP_TOKEN=<from generate-secrets>
#   VNMS_HTTP_PUBLISH=10.0.1.10:8080:8080   # this VM's private IP
#   VSRV_PRIVATE_IP=10.0.1.20               # VSRV private IP (firewall)

VSRV_PRIVATE_IP=10.0.1.20 ./scripts/02-firewall.sh
./scripts/03-deploy.sh
./scripts/smoke-test.sh
```

### Import factory devices

```sh
# Example CSV: device_id,device_key_hex
echo 'device_id,device_key_hex
860123456789012,000102030405060708090a0b0c0d0e0f' > /tmp/devices.csv

source .env
./scripts/import-inventory.sh /tmp/devices.csv http://127.0.0.1:8080 "$VNMS_HTTP_TOKEN"
```

Optional admin UI: see `vigelo-nms/deploy/README.md` (`COMPOSE_PROFILES=admin`).

## 3. Deploy VSRV (vm-vsrv)

```sh
cd vigelo-backend
SERVER=root@<vsrv-public-ip> ./deploy/scripts/sync-from-local.sh

# On VSRV server
cd /opt/vigelo-backend/deploy
./scripts/01-install-docker.sh
cp .env.example .env
# Edit .env:
#   POSTGRES_PASSWORD
#   VSRV_HOSTNAME=api.yourdomain.com
#   VSRV_PUBLIC_URL=https://api.yourdomain.com
#   FRONTEND_BASE_URL=https://app.yourdomain.com
#   VSRV_CORS_ORIGIN=https://app.yourdomain.com
#   VSRV_ADMIN_EMAILS=you@example.com
#   VNMS_BASE_URL=http://10.0.1.10:8080
#   VNMS_HTTP_TOKEN=<same as VNMS>

./scripts/02-firewall.sh
./scripts/03-deploy.sh
./scripts/check-vnms.sh
./scripts/smoke-test.sh
```

## 4. Verify integration

1. Open `https://<VSRV_HOSTNAME>/admin/` → **Status** tab
2. **Database**: ok
3. **VNMS**: ok (live health check to VNMS `/healthz`)
4. **MailerSend**: fail until domain verified (expected until configured)

VSRV background workers (automatic when VNMS is configured):

- VNMS event consumer (10s poll) — movement/offline events
- Offline checker (5 min)
- Trial expiry → VNMS disable

## 5. Test device claim

```sh
# Register user (replace URL/email/password)
curl -s -X POST https://api.yourdomain.com/v1/auth/signup \
  -H 'Content-Type: application/json' \
  -d '{"email":"test@example.com","password":"secret123","display_name":"Test"}'

# Login, create household, register device (needs valid enrollment secret from VNMS inventory)
# Or use admin console → provision device under household
```

## Single-server shortcut (dev only)

For initial testing on **one** UpCloud host, run both stacks:

- VNMS: `VNMS_HTTP_PUBLISH=127.0.0.1:8080:8080`
- VSRV: `VNMS_BASE_URL=http://172.17.0.1:8080` or host gateway IP

Production should use two VMs with private-network firewall rules.

## TLS between VSRV and VNMS (optional)

Default: HTTP over private network + ufw restricting port 8080 to VSRV IP.

For HTTPS: terminate TLS on VNMS with an internal CA, place `ca.pem` in
`vigelo-backend/deploy/certs/`, set `VNMS_TLS_CA=/certs/ca.pem` and
`VNMS_BASE_URL=https://...`.

## Related docs

- [`deploy/README.md`](../deploy/README.md) — VSRV deploy details
- [`vnms-integration.md`](vnms-integration.md) — API contracts
- `vigelo-nms/deploy/README.md` — VNMS deploy details
- [`implementation-status.md`](implementation-status.md) — feature status
