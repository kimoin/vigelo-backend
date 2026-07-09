# VSRV Implementation Status

Last updated: 2026-07-09

This document records what is **implemented and verified** in `vigelo-backend`
(VSRV) and the related VNMS changes through **Phase 5**, plus the monitored-window
**alert mode** preference (movement *or* no-movement, not both).

For planning and locked-in decisions, see [`vsrvplan.md`](vsrvplan.md). For API
shape direction, see [`mobile-api.md`](mobile-api.md).

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
| `vigelo-backend` | Product server (VSRV) | Phases 1–5 complete |
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
| **6** | Push persistence, trial expiry job, notification dispatcher | Not started |
| **7** | UpCloud deploy, factory E2E | Not started |

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
    httpapi/                    # HTTP routes and handlers
    store/
      postgres/                 # Users, sessions, households, devices, alerts, events
      memory/                   # Push tokens + legacy helpers (alerts removed from memory)
    devices/                    # Register, list, monitored windows, VNMS merge
    vnmsclient/                 # VNMS HTTP client
    vnmsevents/                 # Event poll + offline checker
    alerts/                     # VNMS event → alert instances
    schedule/                   # Local-time → UTC window conversion
    notifications/
      email/                    # MailerSend + log sender
      sms/                      # GatewayAPI + log sender
      dispatcher.go             # Critical alert SMS routing
  migrations/                   # 001–004 SQL
  deploy/                       # Docker Compose, Caddy, .env.example
  docs/
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

### Persistence by feature

| Feature | Storage |
|---------|---------|
| Users, sessions, households, invites | PostgreSQL |
| Device bindings, subscriptions | PostgreSQL |
| Monitored window intent + alert_mode | PostgreSQL |
| Alert rules + alert instances | PostgreSQL |
| VNMS event cursor + processed events | PostgreSQL |
| Notification delivery log | PostgreSQL |
| Push tokens | In-memory (Phase 6: Postgres) |
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

### Push (in-memory)

| Method | Path |
|--------|------|
| POST | `/v1/push-tokens` |
| DELETE | `/v1/push-tokens/{push_token_id}` |

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

---

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

### SMS (`internal/notifications/sms/`)

- **GatewayAPI** when `GATEWAYAPI_TOKEN` is set
- Log sender otherwise
- SMS sent for critical types (`no_movement_detected`, `device_offline`) when rule
  includes `sms` and household member has **verified** phone

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
| Push token Postgres + real push provider | Phase 6 |
| Trial expiry → VNMS disable job | Phase 6 |
| Payment provider | Fake checkout URL only |
| Per-device notification preferences API | Alert mode on monitored windows only |
| Quiet hours | Schema exists on `alert_rules`; not evaluated |
| VNMS activity/timeline in mobile API | Not wired |
| UpCloud production deploy | Phase 7 |
| Kubernetes | Deferred until pre-launch |

---

## Related Documents

Keep these aligned when behavior changes:

- [`vsrvplan.md`](vsrvplan.md) — master plan and decisions
- [`vnms-integration.md`](vnms-integration.md) — VNMS contracts
- [`device-lifecycle.md`](device-lifecycle.md) — enrollment flow
- [`mobile-api.md`](mobile-api.md) — mobile API shape
- [`alerts-notifications.md`](alerts-notifications.md) — alert and SMS behavior
- [`development-plan.md`](development-plan.md) — original phase ordering
