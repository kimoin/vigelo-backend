# VSRV Implementation Status

Last updated: 2026-07-10

This document records what is **implemented and verified** in `vigelo-backend`
(VSRV) through **Phase 7 deploy readiness**, including the admin console,
audit logging, live service health checks, and UpCloud deployment scripts.

For operations and deployment, see [`operations.md`](operations.md) and
[`upcloud-deploy.md`](upcloud-deploy.md). For admin UI details, see
[`admin-console.md`](admin-console.md).

## System Context

```text
vigelo-frontend (web prototype)
  -> VSRV (vigelo-backend) :8090
       -> PostgreSQL (device bindings, users, alerts, events cursor)
       -> VNMS (vigelo-nms) :8080  [HTTPS + bearer token]
            -> Vigelo device (NB-IoT UDP)
```

| Repository | Role | Phase 1–5 status |
|------------|------|------------------|
| `vigelo-backend` | Product server (VSRV) | Phases 1–7 deploy ready; admin console |
| `vigelo-nms` | Device network server (VNMS) | Enrollment + inventory APIs |
| `vigelo-frontend` | Mobile UI prototype | Auth, invites, devices, monitored hours |
| `vigelo-app` | Native app | Placeholder |
| `vigelo-device` | Hardware/firmware | Design + PoC |

## Phase Summary

| Phase | Goal | Status |
|-------|------|--------|
| **1** | Skeleton, Docker, Postgres migrations, modular Go layout | Done |
| **2** | Auth, households, invites, MailerSend, role-based authz | Done |
| **3** | VNMS client, device enrollment, Postgres device bindings | Done |
| **4** | Monitored windows, timezone, UTC conversion, policy delivery | Done |
| **5** | VNMS event consumer, Postgres alerts, offline detection, SMS | Done |
| **6** | Push, trial expiry, audit, admin console | Done |
| **7** | UpCloud deploy, factory E2E | Deploy scripts + docs; see `docs/upcloud-deploy.md` |

---

## VSRV Codebase Layout

```text
vigelo-backend/
  cmd/vsrv/main.go              # Entry point, wires DB, VNMS, SMS, background workers
  internal/
    config/                     # Env loading
    logging/
    domain/                     # API models + alert_mode constants
    ids/                        # Opaque ID generation
    auth/                       # Argon2id, session errors
    authz/                      # Household roles
    adminweb/                   # Embedded admin UI at /admin/
    adminstatus/                # Live service health checks
    audit/                      # Audit logger + retention
    trials/                     # Trial expiry worker
    httpapi/                    # HTTP routes (incl. admin_handlers.go)
    store/
      postgres/                 # All persistence incl. admin, audit, push_tokens
    devices/                    # Register, list, monitored windows, VNMS merge
    vnmsclient/                 # VNMS HTTP client
    vnmsevents/                 # Event poll + offline checker
    alerts/                     # VNMS event → alert instances
    schedule/                   # Local-time → UTC window conversion
    notifications/
      email/                    # MailerSend + log + health check
      sms/                      # GatewayAPI + log + health check
      push/                     # log / ntfy / apns stub
      dispatcher.go             # Push + SMS + audit on delivery
  migrations/                   # 001–005 SQL
  deploy/                       # Docker Compose, Caddy, scripts, README
  docs/
    operations.md               # Operations guide
    upcloud-deploy.md           # Two-VM UpCloud guide
    admin-console.md            # Admin UI reference
```

---

## Database (PostgreSQL)

Migrations apply in order via `make migrate` or `make migrate-docker`.

| Migration | Contents |
|-----------|----------|
| `001_initial_schema.sql` | Full domain: users, households, device_bindings, monitored_window_intent, subscriptions, alert_rules, alerts, push_tokens, vnms_event_cursor, processed_vnms_events, audit_log |
| `002_sessions_access_token.sql` | Access token on sessions |
| `003_device_projection_alerts.sql` | `device_bindings.last_contact_at`, alert idempotency indexes |
| `004_monitored_window_alert_mode.sql` | `monitored_window_intent.alert_mode` |
| `005_audit_message.sql` | `audit_log.message` column + index |

### Persistence by feature

| Feature | Storage |
|---------|---------|
| Users, sessions, households, invites | PostgreSQL |
| Device bindings, subscriptions | PostgreSQL |
| Monitored window intent + alert_mode | PostgreSQL |
| Alert rules + alert instances | PostgreSQL |
| VNMS event cursor + processed events | PostgreSQL |
| Notification delivery log | PostgreSQL |
| Push tokens | PostgreSQL |
| Push delivery | `log` / `ntfy` / `apns` (stub) via `PUSH_PROVIDER` |
| SMS | GatewayAPI when `GATEWAYAPI_TOKEN` set; log fallback otherwise |
| Activity timeline | Stub data (Phase 5+: VNMS) |

---

## HTTP API (implemented)

Base URL: `http://127.0.0.1:8090` (local). All `/v1/*` routes except auth signup/login
require `Authorization: Bearer <access_token>`.

### Auth

| Method | Path | Notes |
|--------|------|-------|
| POST | `/v1/auth/signup` | Creates user + default household; sends verify email |
| POST | `/v1/auth/login` | Returns access + refresh tokens |
| POST | `/v1/auth/refresh` | Rotates tokens |
| POST | `/v1/auth/logout` | Revokes session |
| POST | `/v1/auth/verify-email` | Email verification token |
| POST | `/v1/auth/password-reset/request` | Sends reset email |
| POST | `/v1/auth/password-reset/complete` | Sets new password |
| GET | `/v1/me` | Current user |
| PATCH | `/v1/me` | `display_name`, `timezone` |

### Households

| Method | Path | Notes |
|--------|------|-------|
| GET | `/v1/households` | List memberships |
| POST | `/v1/households` | Create household |
| PATCH | `/v1/households/{household_id}` | Update `name`, `timezone` (owner/admin) |
| GET | `/v1/households/{household_id}/members` | List members |
| POST | `/v1/households/{household_id}/invites` | Invite caregiver (MailerSend) |
| POST | `/v1/invites/{token}/accept` | Accept invite |

### Devices

| Method | Path | Notes |
|--------|------|-------|
| GET | `/v1/households/{household_id}/devices` | List; merges VNMS state via batchGet |
| POST | `/v1/households/{household_id}/devices/register` | Structured enrollment |
| POST | `/v1/households/{household_id}/device-claims` | QR alias (`qr_payload`) |
| GET | `/v1/devices/{device_binding_id}` | Detail + VNMS projection |
| PATCH | `/v1/devices/{device_binding_id}` | `display_name`, `room_or_location_label` |
| POST | `/v1/devices/{device_binding_id}/remove` | Soft-remove binding |
| GET | `/v1/devices/{device_binding_id}/monitored-windows` | Local intent + `alert_mode` |
| PUT | `/v1/devices/{device_binding_id}/monitored-windows` | Save windows, sync to VNMS |
| GET | `/v1/devices/{device_binding_id}/activity` | **Stub** (demo data) |
| GET | `/v1/devices/{device_binding_id}/alerts` | Postgres-backed alerts |
| POST | `/v1/devices/{device_binding_id}/alerts/{alert_id}/ack` | Acknowledge alert |
| GET | `/v1/devices/{device_binding_id}/subscription` | Subscription state |
| POST | `/v1/devices/{device_binding_id}/subscription/checkout` | Demo activation (no payment provider) |

### Push (PostgreSQL)

| Method | Path |
|--------|------|
| POST | `/v1/push-tokens` |
| DELETE | `/v1/push-tokens/{push_token_id}` |

### Admin (requires `VSRV_ADMIN_EMAILS`)

Embedded UI at `/admin/`. API under `/v1/admin/*`. See [`admin-console.md`](admin-console.md).

### Health

| Method | Path |
|--------|------|
| GET | `/healthz` |

---

## Environment Variables

See `.env.example` and `deploy/.env.example`.

| Variable | Purpose |
|----------|---------|
| `VSRV_ADDR` | Listen address (default `127.0.0.1:8090`) |
| `VSRV_DATABASE_URL` | **Required** Postgres URL |
| `VSRV_CORS_ORIGIN` | Comma-separated frontend origins |
| `FRONTEND_BASE_URL` | Invite and email links |
| `OFFLINE_THRESHOLD_HOURS` | Offline alert threshold (default `3`) |
| `DEFAULT_TRIAL_DAYS` | Trial length on enrollment (default `30`) |
| `MAILERSEND_API_TOKEN` | Email; logs only if unset |
| `VNMS_BASE_URL` | VNMS HTTP base; enrollment disabled if unset |
| `VNMS_HTTP_TOKEN` | Bearer token for VNMS |
| `VNMS_TLS_CA` | Optional CA file for private-network TLS |
| `GATEWAYAPI_TOKEN` | SMS; logs only if unset |
| `GATEWAYAPI_SENDER` | SMS sender name |
| `PUSH_PROVIDER` | `log` (default), `ntfy`, or `apns` |
| `NTFY_BASE_URL` | ntfy server (default `https://ntfy.sh`) |
| `NTFY_TOKEN` | Optional ntfy auth token |
| `APNS_KEY_ID` | Apple push (when native app ready) |
| `APNS_TEAM_ID` | Apple Developer team ID |
| `APNS_KEY_PATH` | Path to `.p8` auth key |
| `APNS_BUNDLE_ID` | iOS app bundle ID |
| `APNS_SANDBOX` | `true` for development builds |
| `VSRV_ADMIN_EMAILS` | Comma-separated admin console emails |
| `VSRV_AUDIT_RETENTION_DAYS` | Audit log retention (default `60`) |
| `VSRV_ACCESS_TOKEN_TTL_HOURS` | Access token TTL (default `1`) |
| `VSRV_REFRESH_TOKEN_TTL_DAYS` | Refresh token TTL (default `30`) |

Full reference: [`operations.md`](operations.md).

## Phase 1 — Skeleton and Docker

- Modular `internal/*` layout extracted from monolithic MVP
- `docker-compose.yml`: Postgres on port **5433**
- `deploy/`: production-style Compose + Caddyfile
- `Makefile`: `db-up`, `migrate`, `migrate-docker`, `run`, `test`
- Structured logging, config loading, CORS
- `GET /healthz` with database ping

---

## Phase 2 — Auth, Households, Invites

- **Argon2id** password hashing
- Access + refresh token rotation (`002` migration)
- PostgreSQL: users, sessions, households, members, invites
- Roles: `owner`, `admin`, `caregiver`, `member` (`internal/authz`)
- MailerSend integration with log fallback
- Frontend: `/invite/{token}` accept flow in `vigelo-frontend`

---

## Phase 3 — VNMS Client and Enrollment

### VNMS (`vigelo-nms`) additions

| Endpoint | Purpose |
|----------|---------|
| `POST /v1/devices:provision-inventory` | Factory import as `disabled` |
| `POST /v1/devices/{device_id}/verify-enrollment` | Constant-time key check |
| `deploy/scripts/import-inventory.sh` | CSV batch import script |

### VSRV

- `internal/vnmsclient/`: verify-enrollment, enable, batchGet
- Postgres `device_bindings` + trialing `subscriptions` on register
- Enrollment flow: verify → bind → trial → VNMS enable → batchGet merge
- `POST .../devices/register` and `POST .../device-claims` (QR alias)
- Device list/detail reads VNMS state; bindings stored in Postgres

### Enrollment API

```json
POST /v1/households/{household_id}/devices/register
{
  "device_id": "860123456789012",
  "enrollment_secret": "000102030405060708090a0b0c0d0e0f",
  "display_name": "Kitchen",
  "room_or_location_label": "Home",
  "timezone": "Europe/Helsinki"
}
```

QR alias (`device-claims`): `device_id=...&key=<32-hex-chars>`

Raw device keys are **not** persisted after successful enrollment.

---

## Phase 4 — Monitored Windows and Timezone

- `internal/schedule/`: validate local windows, convert to VNMS UTC policy (max 2 UTC windows)
- `PUT /monitored-windows` → VNMS `PUT /v1/devices/{device_id}/monitored-windows`
- Delivery states: `not_configured`, `pending_delivery`, `delivered`, `sync_failed`
- `device.policy_delivered` events + VNMS state match clear pending delivery
- `PATCH /v1/households/{id}` for household timezone
- `PATCH /v1/me` for user timezone
- Activity endpoint uses device/household timezone (data still stubbed)

### Monitored windows payload

```json
PUT /v1/devices/{device_binding_id}/monitored-windows
{
  "timezone": "Europe/Helsinki",
  "windows": [
    { "start_time": "08:00", "end_time": "20:00" }
  ],
  "alert_mode": "no_movement_detected"
}
```

`alert_mode` is documented in the **Alert preference** section below.

---

## Phase 5 — Events, Alerts, Offline, SMS

### Background workers (`internal/vnmsevents/`)

- Polls VNMS `GET /v1/events?after={cursor}&limit=100` every **10s**
- Offline check every **5 min** using `OFFLINE_THRESHOLD_HOURS`
- Idempotent processing via `processed_vnms_events`
- Cursor in `vnms_event_cursor`

### VNMS events handled

| Event | VSRV action |
|-------|-------------|
| `monitored_window.movement_detected` | Create movement alert (if rule enabled) |
| `monitored_window.no_movement_detected` | Create no-movement alert + optional SMS |
| `movement_uplink.accepted` | Update `last_contact_at`; resolve offline alerts |
| `device_status.received` | Update contact + voltage projection |
| `device.policy_delivered` | Mark monitored windows delivered |
| `device.lifecycle_changed` | Update `vnms_lifecycle_cache` |

### Alert types (MVP)

| Type | Default rule | Channels |
|------|--------------|----------|
| `movement_detected` | Disabled until user chooses | push |
| `no_movement_detected` | Disabled until user chooses | push, sms |
| `device_offline` | Enabled | push, sms |

Alerts are stored in Postgres. `GET /alerts` and `POST .../ack` use the database.

### Push (`internal/notifications/push/`)

- **Log** (default) when `PUSH_PROVIDER=log`
- **ntfy** when `PUSH_PROVIDER=ntfy` — token is the topic name
- **APNs stub** when `PUSH_PROVIDER=apns` — set `APNS_*` env vars; wire `apns2` when native iOS app ships
- Dispatcher sends push for any alert rule that includes `push` channel
- Tokens stored in Postgres (`push_tokens`); register via `POST /v1/push-tokens`

### SMS (`internal/notifications/sms/`)

- **GatewayAPI** when `GATEWAYAPI_TOKEN` is set
- Log sender otherwise — **no code changes needed; add credentials only**
- SMS sent for critical types (`no_movement_detected`, `device_offline`) when rule
  includes `sms` and household member has **verified** phone

---

## Phase 6 — Admin Console, Audit, Push, Trials

### Admin console (`internal/adminweb/`)

- Embedded UI at `/admin/` — Users, Status, Audit log, Dashboard
- Auth via normal login + `VSRV_ADMIN_EMAILS` allowlist
- User drill-down: households, members (invite status), devices, subscriptions
- Device ops: VNMS enable/disable, provision, trial extend, move, delete
- Live service health: Database, VNMS, MailerSend, GatewayAPI, ntfy

See [`admin-console.md`](admin-console.md).

### Audit logging (`internal/audit/`, migration `005`)

- `audit_log` table with human-readable `message` field
- Retention job (`VSRV_AUDIT_RETENTION_DAYS`)
- Logs: auth, device, admin, notification, trial events

### Trial expiry (`internal/trials/`)

- Background worker suspends expired trials
- Calls VNMS disable when trial ends

### Push persistence

- `push_tokens` in PostgreSQL (memory store removed)
- Providers: `log`, `ntfy`, `apns` (stub)

---

## Phase 7 — UpCloud Deployment

- `deploy/docker-compose.yml` — postgres, migrate, vsrv, caddy (full env)
- `deploy/scripts/` — install, firewall, deploy, sync, smoke-test, check-vnms
- `deploy/README.md` + [`upcloud-deploy.md`](upcloud-deploy.md)
- VNMS integration: shared `VNMS_HTTP_TOKEN`, private-network HTTP
- Dockerfile includes migrations for runtime auto-migrate

---

## Alert Preference (movement OR no-movement)

Users choose **one** monitored-window alert mode when saving monitored hours.
The modes are mutually exclusive.

| `alert_mode` | Meaning |
|--------------|---------|
| `no_movement_detected` | Alert if **no** movement during monitored window (default) |
| `movement_detected` | Alert if movement **is** detected during monitored window |

Implementation:

- Stored on `monitored_window_intent.alert_mode` (migration `004`)
- On `PUT /monitored-windows`, VSRV enables exactly one movement-related
  `alert_rules` row and disables the other
- VNMS event handler checks `alert_rules.enabled` before creating an alert
- Frontend: radio buttons on monitored hours form (`vigelo-frontend`)

`device_offline` alerts are independent of this choice.

---

## Local Development

### Prerequisites

- Go 1.26+
- Docker Desktop (for Postgres)
- VNMS running locally for full enrollment flow

### VSRV

```bash
cd vigelo-backend
cp .env.example .env   # add MAILERSEND / VNMS tokens as needed
make db-up
make migrate-docker
make run
```

### VNMS (separate terminal)

```bash
cd vigelo-nms
go run ./cmd/vnms
```

### Factory inventory (before user enrollment)

```bash
# Or use deploy/scripts/import-inventory.sh
curl -X POST http://127.0.0.1:8080/v1/devices:provision-inventory \
  -H "Content-Type: application/json" \
  -d '{"device_id":"860123456789012","device_key":"000102030405060708090a0b0c0d0e0f"}'
```

### Frontend

```bash
cd vigelo-frontend
npm run dev
```

### End-to-end checklist

1. Sign up / log in
2. Create or select household
3. Provision device in VNMS (inventory)
4. Claim device (`device-claims` or `devices/register` with real 32-char hex key)
5. Activate subscription (demo checkout)
6. Set monitored hours + choose alert mode
7. VNMS emits events → alerts appear in app
8. Invite caregiver via `/invite/{token}`

---

## What Is Not Done Yet

| Area | Notes |
|------|-------|
| OpenAPI spec for VSRV | Planned; routes documented here |
| Activity from VNMS | `GET /activity` returns stub data |
| Push token Postgres + push providers | Done (`log`/`ntfy`; APNs stub for later) |
| Trial expiry → VNMS disable job | Phase 6 |
| Payment provider | Fake checkout URL only |
| Per-device notification preferences API | Alert mode on monitored windows only |
| Quiet hours | Schema exists on `alert_rules`; not evaluated |
| VNMS activity/timeline in mobile API | Not wired |
| UpCloud production deploy | `deploy/` + `docs/upcloud-deploy.md` (two-VM VSRV+VNMS) |
| Kubernetes | Deferred until pre-launch |

---

## Related Documents

Keep these aligned when behavior changes:

- [`operations.md`](operations.md)
- [`upcloud-deploy.md`](upcloud-deploy.md)
- [`admin-console.md`](admin-console.md)

- [`vsrvplan.md`](vsrvplan.md) — master plan and decisions
- [`vnms-integration.md`](vnms-integration.md) — VNMS contracts
- [`device-lifecycle.md`](device-lifecycle.md) — enrollment flow
- [`mobile-api.md`](mobile-api.md) — mobile API shape
- [`alerts-notifications.md`](alerts-notifications.md) — alert and SMS behavior
- [`development-plan.md`](development-plan.md) — original phase ordering
