# VSRV Operations Guide

Complete reference for running VSRV locally and on UpCloud, including VNMS
integration, configuration, and verification.

## Repositories

| Repo | Role | Deploy path |
|------|------|-------------|
| `vigelo-backend` | VSRV — product API, users, alerts, admin | `deploy/` |
| `vigelo-nms` | VNMS — device UDP, enrollment, events | `deploy/` |
| `vigelo-frontend` | Web prototype | Static / dev server |

See [`upcloud-deploy.md`](upcloud-deploy.md) for the two-VM UpCloud topology.

---

## Local development

### Prerequisites

- Go 1.26+
- Docker (for Postgres)
- Optional: running VNMS on port 8080

### Start Postgres

```sh
make db-up          # Postgres on localhost:5433
make migrate-docker # Apply migrations 001–005
```

### Configure environment

```sh
cp .env.example .env
# Edit: VSRV_DATABASE_URL, VNMS_BASE_URL, VNMS_HTTP_TOKEN, VSRV_ADMIN_EMAILS
```

### Run VSRV

```sh
make run
# Listens on VSRV_ADDR (default 127.0.0.1:8090)
```

### Verify

```sh
curl -s http://127.0.0.1:8090/healthz
open http://127.0.0.1:8090/admin/
```

Migrations also run automatically at startup from `/app/migrations` (Docker) or
`migrations/` (local `make run` from repo root).

---

## Environment variables (complete)

### Core

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `VSRV_ADDR` | No | `127.0.0.1:8090` | HTTP listen address |
| `VSRV_DATABASE_URL` | **Yes** | — | PostgreSQL connection URL |
| `VSRV_PUBLIC_URL` | Prod | — | Public API URL (emails, links) |
| `FRONTEND_BASE_URL` | Prod | `http://127.0.0.1:5173` | Frontend URL for invite links |
| `VSRV_CORS_ORIGIN` | No | localhost origins | Comma-separated CORS origins |
| `VSRV_LOG_LEVEL` | No | `info` | Log level |

### VNMS integration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `VNMS_BASE_URL` | For devices | — | VNMS HTTP base URL |
| `VNMS_HTTP_TOKEN` | Prod | — | Bearer token (must match VNMS server) |
| `VNMS_TLS_CA` | No | — | Path to CA PEM for private-network HTTPS |

Without `VNMS_BASE_URL`, auth and households work; device enrollment returns
service unavailable.

### Email (MailerSend)

| Variable | Default | Description |
|----------|---------|-------------|
| `MAILERSEND_API_TOKEN` | — | API token; logs only if unset |
| `MAILERSEND_FROM_EMAIL` | `notify@vigelo.fi` | From address (domain must be verified) |
| `MAILERSEND_FROM_NAME` | `Vigelo` | From display name |

### SMS (GatewayAPI)

| Variable | Default | Description |
|----------|---------|-------------|
| `GATEWAYAPI_TOKEN` | — | API token; logs only if unset |
| `GATEWAYAPI_SENDER` | `Vigelo` | SMS sender name |

### Push notifications

| Variable | Default | Description |
|----------|---------|-------------|
| `PUSH_PROVIDER` | `log` | `log`, `ntfy`, or `apns` |
| `NTFY_BASE_URL` | `https://ntfy.sh` | ntfy server URL |
| `NTFY_TOKEN` | — | Optional ntfy auth |
| `APNS_KEY_ID` | — | Apple push (future) |
| `APNS_TEAM_ID` | — | Apple Developer team ID |
| `APNS_KEY_PATH` | — | Path to `.p8` key |
| `APNS_BUNDLE_ID` | — | iOS bundle ID |
| `APNS_SANDBOX` | `false` | APNs sandbox mode |

### Admin console

| Variable | Default | Description |
|----------|---------|-------------|
| `VSRV_ADMIN_EMAILS` | — | Comma-separated admin emails |
| `VSRV_AUDIT_RETENTION_DAYS` | `60` | Audit log retention |

### Policy / trials

| Variable | Default | Description |
|----------|---------|-------------|
| `OFFLINE_THRESHOLD_HOURS` | `3` | Hours before offline alert |
| `DEFAULT_TRIAL_DAYS` | `30` | Trial length on device enrollment |

### Session TTLs

| Variable | Default |
|----------|---------|
| `VSRV_ACCESS_TOKEN_TTL_HOURS` | `1` |
| `VSRV_REFRESH_TOKEN_TTL_DAYS` | `30` |
| `VSRV_INVITE_TTL_DAYS` | `7` |
| `VSRV_VERIFY_EMAIL_TTL_HOURS` | `48` |
| `VSRV_RESET_PASSWORD_TTL_HOURS` | `2` |

---

## Background workers

Started automatically when Postgres and VNMS are configured:

| Worker | Interval | Purpose |
|--------|----------|---------|
| VNMS event consumer | 10s | Poll VNMS events → alerts |
| Offline checker | 5 min | Detect devices past contact threshold |
| Trial expiry | 1h | Suspend expired trials (VNMS disable) |
| Audit retention | 24h | Prune audit log older than retention days |

---

## UpCloud production deploy

### Overview

```
vm-vnms (private 10.x.x.10)          vm-vsrv (public HTTPS)
  UDP 5642 ← devices                    Caddy :443 → vsrv
  HTTP 8080 ← VSRV only (ufw)           postgres internal
```

### Step-by-step

1. **Generate secrets** (local):
   ```sh
   cd vigelo-backend/deploy && ./scripts/generate-secrets.sh
   ```

2. **Deploy VNMS** — see `vigelo-nms/deploy/README.md` and [`upcloud-deploy.md`](upcloud-deploy.md)

3. **Deploy VSRV** — see `deploy/README.md`

4. **Verify integration**:
   ```sh
   ./scripts/check-vnms.sh    # on VSRV server
   ./scripts/smoke-test.sh
   ```
   Admin → Status tab: Database **ok**, VNMS **ok**

### Factory device import

On VNMS server:

```sh
./scripts/import-inventory.sh devices.csv http://127.0.0.1:8080 "$VNMS_HTTP_TOKEN"
```

CSV format: `device_id,device_key_hex` (see `deploy/example-devices.csv`).

### Device enrollment flow

1. Factory import in VNMS (device `disabled` in inventory)
2. User registers on VSRV
3. User claims device with `device_id` + enrollment secret
4. VSRV: `verify-enrollment` → Postgres binding + trial → VNMS `enable`
5. Device appears in admin console with trial/subscription status

---

## Database migrations

| File | Contents |
|------|----------|
| `001_initial_schema.sql` | Full schema |
| `002_sessions_access_token.sql` | Session access tokens |
| `003_device_projection_alerts.sql` | Device contact + alert indexes |
| `004_monitored_window_alert_mode.sql` | Alert mode on monitored windows |
| `005_audit_message.sql` | `audit_log.message` column |

Apply: `make migrate-docker` (local) or migrate service in `deploy/docker-compose.yml`.

---

## Troubleshooting

| Issue | Resolution |
|-------|------------|
| `VSRV_DATABASE_URL is required` | Set in `.env` or deploy `.env` |
| Device enrollment unavailable | Set `VNMS_BASE_URL`; check VNMS health |
| VNMS 401 from VSRV | `VNMS_HTTP_TOKEN` must match on both servers |
| Caddy cert fails | DNS for `VSRV_HOSTNAME` must point to server |
| Admin login works but tabs 403 | Add email to `VSRV_ADMIN_EMAILS` |
| Push/SMS not sent | Check Status tab; tokens may be log-only |

---

## Related documents

- [`admin-console.md`](admin-console.md) — Admin UI and API
- [`upcloud-deploy.md`](upcloud-deploy.md) — Two-VM UpCloud guide
- [`vnms-integration.md`](vnms-integration.md) — VNMS API contracts
- [`implementation-status.md`](implementation-status.md) — Feature status
- `deploy/README.md` — VSRV deploy scripts
