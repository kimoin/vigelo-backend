# VSRV Implementation Plan

## Purpose

This document is the consolidated implementation plan for Vigelo Backend (VSRV):
the application server between Vigelo NMS (VNMS) and the end-user mobile client.
It captures architecture decisions, deployment topology, integration contracts,
and phased delivery scope agreed for the first production-oriented MVP.

VSRV owns the product plane. VNMS owns the device-network plane. The mobile app
talks only to VSRV.

For module boundaries and entity definitions, see also:

- [`architecture.md`](architecture.md)
- [`domain-model.md`](domain-model.md)
- [`vnms-integration.md`](vnms-integration.md)
- [`mobile-api.md`](mobile-api.md)
- [`development-plan.md`](development-plan.md)

## System Context

```text
Mobile client (vigelo-frontend for now, native app later)
  -> VSRV (Go, PostgreSQL, UpCloud)
     -> VNMS (Go, PostgreSQL, UpCloud) over HTTPS on private network
        -> Vigelo device (NB-IoT UDP)
```

| Component | Repository | Role |
|-----------|------------|------|
| VNMS | `vigelo-nms` | UDP ingress, AEAD, hourly movement, monitored-window facts, outbox events |
| VSRV | `vigelo-backend` | Users, households, enrollment, subscriptions, alerts, notifications |
| Mobile UI | `vigelo-frontend` | Web prototype until native `vigelo-app` is started |
| Device | `vigelo-device` | Hardware, firmware, protocol specs |

## Locked-In Decisions

| Topic | Decision |
|-------|----------|
| Language | Go (same as VNMS) |
| Cloud | UpCloud, European footprint |
| VSRV persistence | PostgreSQL with SQL migrations |
| MVP deployment | Docker Compose per VM (not Kubernetes until pre-launch) |
| VM topology | Separate VMs and databases for VNMS and VSRV from day one |
| VSRV codebase | Refactor existing in-memory MVP incrementally |
| Mobile client | Continue `vigelo-frontend`; focus on server first |
| Multi-tenancy | Multi-household, multi-device, multi-caregiver from the beginning |
| Device enrollment | API-based registration; QR format defined later at factory |
| Factory secrets | Generated at manufacturing; secrets file imported by VNMS admin |
| Pre-enrollment state | Device exists in VNMS before user registers |
| Subscription inactive | VNMS stops device processing (`disable`), not VSRV-only |
| Timezone | User-settable; household default; used for monitored hours |
| Offline alert | Configurable parameter; default 3 hours |
| No-movement grace | Handled by NMS monitoring-window delay; no extra VSRV grace |
| Payments | After working MVP; pluggable provider interface; no provider chosen yet |
| Trials / campaigns | Schema and hooks ready; configurable trial period |
| Email | MailerSend (EU-based) for verification, invites, password reset |
| SMS | GatewayAPI for critical alerts; phone optional until user opts in |
| Push | MVP dispatcher + token registry; EU-friendly provider TBD until native app |
| VSRV ↔ VNMS transport | HTTPS from the start on UpCloud private network |
| TLS termination | Caddy on both VMs |

## Service Ownership

### VSRV owns

- User accounts, sessions, email verification, password reset
- Households, memberships, roles, caregiver invites
- Device bindings (product ownership of `device_id`)
- Local-time monitored-hour intent and timezone conversion
- Subscription and service entitlement state (billing details later)
- Alert rules, alert instances, notification routing
- Push token registry and notification delivery log
- Audit log for security-sensitive actions
- Mapping between mobile-facing binding IDs and VNMS `device_id`

### VNMS owns

- UDP device ingress and authenticated downlink
- Per-device key material and replay protection
- Device operational state, telemetry, hourly movement masks
- UTC monitored-window policy as delivered to the device
- Monitored-window movement/no-movement facts
- Durable outbox events for VSRV consumption
- Device lifecycle at the network layer (`active`, `disabled`, `unprovisioned`)

The mobile app must never call VNMS directly.

## Deployment Topology

Two separate UpCloud VMs on a private network (SDN). Each VM runs its own
PostgreSQL instance and Docker Compose stack. Caddy terminates TLS.

```text
┌──────────────────────────── vm-vnms ────────────────────────────┐
│  Caddy :443  ->  vnms :8080 (HTTP API, private)                 │
│  vnms UDP :5642 (carrier / device facing)                       │
│  postgres-vnms                                                  │
└────────────────────────────┬────────────────────────────────────┘
                             │  HTTPS over UpCloud private network
                             │  https://vnms.internal
┌────────────────────────────┴────────────────────────────────────┐
│  vm-vsrv                                                        │
│  Caddy :443  ->  vsrv :8090                                     │
│  postgres-vsrv                                                  │
│  Outbound: MailerSend, GatewayAPI                               │
└────────────────────────────┬────────────────────────────────────┘
                             │  HTTPS public
                      vigelo-frontend
```

### VSRV Docker Compose (planned)

```text
vigelo-backend/deploy/
  docker-compose.yml    # postgres + migrate + vsrv + caddy
  .env.example
  Caddyfile
  scripts/
```

### Environment variables (VSRV)

```bash
VSRV_ADDR=0.0.0.0:8090
VSRV_DATABASE_URL=postgres://vsrv:...@vsrv-db:5432/vsrv
VSRV_PUBLIC_URL=https://api.vigelo.example

VNMS_BASE_URL=https://vnms.internal
VNMS_HTTP_TOKEN=...
VNMS_TLS_CA=/certs/upcloud-ca.pem

MAILERSEND_API_TOKEN=...
MAILERSEND_FROM_EMAIL=notify@vigelo.fi
MAILERSEND_FROM_NAME=Vigelo

GATEWAYAPI_TOKEN=...
GATEWAYAPI_SENDER=Vigelo

OFFLINE_THRESHOLD_HOURS=3
DEFAULT_TRIAL_DAYS=30
FRONTEND_BASE_URL=https://app.vigelo.example

PUSH_PROVIDER=log   # later: apns, unifiedpush, ntfy
```

Kubernetes on UpCloud (UKS) is deferred until close to commercial launch. The
same containers should migrate without architectural change.

## End-to-End Lifecycle

### Factory to shelf

```text
Factory (per device batch)
  -> generate device_id (modem IMEI)
  -> generate device_key (AES-128)
  -> generate QR label (format TBD)
  -> write secrets file

Factory sends secrets file to VNMS admin (secure channel, not git)

VNMS admin imports batch
  -> device_id + device_key stored in VNMS
  -> lifecycle_state = disabled
  -> provisioned = true
  -> device UDP rejected until enabled
```

### Factory secrets file (proposed CSV)

```csv
device_id,device_key_hex,qr_payload_optional,batch_id,manufactured_at
860123456789012,a1b2c3d4e5f6...,vigelo://e/...,BATCH-2026-07-001,2026-07-01
```

- `device_key_hex`: 32 hex characters (16 bytes)
- QR payload format to be finalized with hardware/manufacturing
- Import must not set `lifecycle=active` for inventory devices

### User enrollment

```text
User buys device, opens vigelo-frontend
  -> POST /v1/households/{id}/devices/register
       { device_id, enrollment_secret, display_name, timezone }

VSRV
  -> authorize household membership (devices.claim)
  -> ensure no existing VSRV binding for device_id
  -> VNMS verify-enrollment (key match, disabled, provisioned)
  -> create device_binding
  -> set subscription to trialing
  -> VNMS enable
  -> set monitored windows if provided
  -> discard enrollment_secret (never persist raw device key)

Device becomes active on network; first_contact_at appears after uplink
```

### Terminology mapping

| Product term | VNMS state | VSRV state |
|--------------|------------|------------|
| Factory imported, not enrolled | `provisioned=true`, `lifecycle=disabled` | No binding |
| User enrolled | `lifecycle=active` | `device_binding` created |
| Trial / service active | `lifecycle=active` | `subscription=trialing` or `active` |
| Subscription suspended | `lifecycle=disabled` | `service_suspended` |
| Device removed by user | `unprovisioned` (keys cleared) | `removed_at` set |

## VNMS Integration

### Existing endpoints VSRV uses

```http
GET  /healthz
GET  /v1/devices/{device_id}
POST /v1/devices:batchGet
GET  /v1/devices/{device_id}/activity?start=&end=
PUT  /v1/devices/{device_id}/monitored-windows
POST /v1/devices/{device_id}/enable
POST /v1/devices/{device_id}/disable
POST /v1/devices/{device_id}/unprovision
GET  /v1/events?after=&limit=
```

Authentication: `Authorization: Bearer <VNMS_HTTP_TOKEN>` over HTTPS.

### VNMS extensions required

Today's `POST /v1/devices:provision` always sets `lifecycle=active`. The
factory inventory workflow needs two additions in `vigelo-nms`:

#### 1. Factory batch import (admin)

Import factory CSV into VNMS with:

- `device_id` + `device_key` + derived `key_id`
- `lifecycle_state = disabled`
- audit log entry per batch

Delivery options:

- VNMS admin UI bulk upload, or
- CLI script under `vigelo-nms/deploy/scripts/`

#### 2. Enrollment verification (VSRV-facing)

```http
POST /v1/devices/{device_id}:verify-enrollment
Authorization: Bearer <vnms_token>

{ "device_key_hex": "..." }

200 { "verified": true, "lifecycle_state": "disabled", "provisioned": true }
403  key mismatch
409  device already active (enrolled)
```

Rules:

- Constant-time key comparison against stored key
- Does not change lifecycle state
- VSRV never stores the raw device key after verification

### Subscription to VNMS lifecycle

| VSRV service state | VNMS action |
|--------------------|-------------|
| `service_active` (trial or paid) | `POST .../enable` |
| `service_suspended` / expired | `POST .../disable` |
| Device removed | `POST .../unprovision` |

VNMS `disable` rejects UDP authentication (`lifecycle_state` must be `active` for
key lookup). This satisfies the requirement that inactive subscription stops
device processing at the network layer.

### Event consumer

VSRV polls `GET /v1/events` with a cursor. Delivery is at-least-once; VSRV
deduplicates with `processed_vnms_events`.

Important event types:

- `monitored_window.movement_detected`
- `monitored_window.no_movement_detected`
- `device_status.received`
- `device_info.received`
- `device.policy_delivered`
- `device.lifecycle_changed`
- `movement_uplink.accepted`

VNMS evaluates monitored-window no-movement with built-in delay. VSRV does not
add a separate grace period.

## VSRV API (MVP Focus)

Base path: `/v1`. OpenAPI spec to be published in this repository.

### Authentication

```http
POST /v1/auth/signup
POST /v1/auth/login
POST /v1/auth/refresh
POST /v1/auth/logout
POST /v1/auth/verify-email
POST /v1/auth/password-reset/request
POST /v1/auth/password-reset/complete
GET  /v1/me
PATCH /v1/me                    # includes timezone preference
```

Token model: opaque access token + refresh token with rotation. Password hashing:
Argon2id.

### Households and caregivers

```http
GET  /v1/households
POST /v1/households
GET  /v1/households/{household_id}
PATCH /v1/households/{household_id}
GET  /v1/households/{household_id}/members
POST /v1/households/{household_id}/invites
POST /v1/invites/{token}/accept
```

Invite flow:

1. Owner invites caregiver by email.
2. MailerSend sends `https://app.vigelo.example/invite/{token}`.
3. Invitee signs up or logs in via `vigelo-frontend`.
4. Invitee accepts invite and joins household with assigned role.

### Device registration

```http
POST /v1/households/{household_id}/devices/register
GET  /v1/households/{household_id}/devices
GET  /v1/devices/{device_binding_id}
PATCH /v1/devices/{device_binding_id}
POST /v1/devices/{device_binding_id}/remove
```

Registration request:

```json
{
  "device_id": "860123456789012",
  "enrollment_secret": "<from QR or label>",
  "display_name": "Grandma's bathroom",
  "timezone": "Europe/Helsinki"
}
```

Later, when QR format is frozen, the API may also accept:

```json
{
  "qr_payload": "vigelo://..."
}
```

VSRV parses the payload server-side. The frontend passes the scanned string.

Replaces the in-memory MVP `device-claims` flow, which did not call VNMS.

### Monitored windows and activity

```http
GET /v1/devices/{device_binding_id}/monitored-windows
PUT /v1/devices/{device_binding_id}/monitored-windows
GET /v1/devices/{device_binding_id}/activity
```

VSRV stores local-time user intent, converts to VNMS UTC windows (max two
non-overlapping ranges), and tracks `delivery_state` until
`device.policy_delivered` is received.

### Alerts

```http
GET  /v1/devices/{device_binding_id}/alerts
POST /v1/devices/{device_binding_id}/alerts/{alert_id}/ack
GET  /v1/devices/{device_binding_id}/alert-rules
PUT  /v1/devices/{device_binding_id}/alert-rules
```

Initial alert types:

- `no_movement_detected` (from VNMS events)
- `movement_detected`
- `device_offline` (derived from `last_contact_at`, default threshold 3h)
- `low_battery` (from `device_status.received`)
- `policy_not_delivered`

### Notifications

```http
POST /v1/push-tokens
DELETE /v1/push-tokens/{id}
```

Push delivery is architected in MVP but not fully testable until the native app
exists. Use a provider interface with a log/ntfy dev sink during development.

### Subscriptions (stub until payment provider chosen)

```http
GET  /v1/devices/{device_binding_id}/subscription
POST /v1/devices/{device_binding_id}/subscription/checkout   # 501 or trial-only in MVP
```

## Authorization Model

Every device request resolves through:

```text
authenticated user
  -> household membership
  -> role permission
  -> device binding in that household
  -> subscription/service state where relevant
```

Never authorize using raw `device_id` alone. Mobile APIs use `device_binding_id`.

### Roles (implement from start)

| Role | Permissions |
|------|-------------|
| `owner` | Full access including billing, invites, device register/remove |
| `admin` | Devices, alert rules, members; not billing |
| `caregiver` | View devices, receive and acknowledge alerts |
| `member` | View only |

Permission groups: `household.read`, `household.manage`, `members.manage`,
`devices.claim`, `devices.read`, `devices.configure`, `devices.remove`,
`alerts.read`, `alerts.manage`, `billing.manage`.

## Data Model (PostgreSQL)

Schema must support multi-household, multi-device, and multi-caregiver without
MVP shortcuts baked into constraints.

### Core tables

```text
users
sessions / refresh_tokens
email_verification_tokens
password_reset_tokens

households
household_members
household_invites

device_bindings
monitored_window_intent

subscriptions
subscription_campaigns          # campaign_code, trial_extension_days

alerts
alert_rules
push_tokens
notification_deliveries

vnms_event_cursor
processed_vnms_events

system_config                 # offline_threshold_hours, default_trial_days
audit_log
```

### User phone (optional)

- `users.phone` nullable
- `users.phone_verified_at` required before SMS channel is enabled
- SMS only sent when alert rule includes `sms` and phone is verified

## Notifications

### Email — MailerSend

Use for:

- Email verification on signup
- Password reset
- Caregiver household invites

### SMS — GatewayAPI

Use for critical alerts when configured:

- `no_movement_detected`
- `device_offline`

Respect quiet hours and per-rule channel settings.

### Push (MVP architecture, production provider TBD)

Build `internal/notifications` with a provider interface. Candidates for EU-first
production:

| Option | EU fit | Notes |
|--------|--------|-------|
| Self-hosted ntfy on UpCloud | Excellent | Simple HTTP; useful for dev |
| UnifiedPush | Excellent | Open; requires native app support |
| Apple APNs direct | Good | Required for iOS eventually |
| Firebase FCM | Weak | US Google infrastructure |

MVP: token registration + dispatcher with log/ntfy sink. SMS covers critical
alerts until native push is testable.

## Subscriptions and Payments

Payments are implemented after a working MVP. Design for pluggability now.

### Payment provider interface (planned)

```go
type PaymentProvider interface {
    CreateCheckout(ctx, SubscriptionCheckoutRequest) (CheckoutURL, error)
    CreatePortal(ctx, CustomerID) (PortalURL, error)
    HandleWebhook(ctx, payload []byte, signature string) (WebhookResult, error)
}
```

No provider is chosen yet. Stripe, Paytrail, Adyen, and others remain options for
a later decision.

### MVP subscription behavior

```text
Device registered
  -> subscription.status = trialing
  -> trial_ends_at = now + DEFAULT_TRIAL_DAYS
  -> VNMS enable

Trial ends without payment
  -> service_suspended
  -> VNMS disable

Payment provider added later
  -> webhook updates subscription
  -> VNMS enable/disable accordingly
```

Support `campaign_code` and trial extensions in schema for future promotions.

## Codebase Structure (Target)

Refactor incrementally from `cmd/vsrv/main.go` in-memory MVP:

```text
vigelo-backend/
  cmd/vsrv/main.go
  internal/
    config/
    httpapi/
    auth/
    accounts/
    households/
    devices/
    vnmsclient/
    events/
    alerts/
    notifications/
      email/          # mailersend
      sms/            # gatewayapi
      push/           # provider interface
    subscriptions/
    payments/         # provider interface stub
    store/
    audit/
    logging/
  migrations/
  deploy/
    docker-compose.yml
    Caddyfile
    .env.example
  docs/
    vsrvplan.md       # this document
  Dockerfile
  Makefile
```

Keep API compatibility with `vigelo-frontend` during refactor.

## Implementation Phases

### Phase 1 — Skeleton and Docker (week 1)

- Extract `main.go` into `internal/httpapi` and `internal/store`
- `deploy/docker-compose.yml`: Postgres + migrate + vsrv + Caddy
- Initial SQL migrations for full domain model
- Makefile: `db-up`, `migrate`, `run`, `test`
- Health endpoint, structured logging, config loading

### Phase 2 — Auth, households, invites (week 2)

- Argon2id password hashing, refresh token rotation
- Households, members, roles, authorization middleware
- MailerSend: verification and invite emails
- Frontend invite route: `/invite/{token}`

### Phase 3 — VNMS client and enrollment (week 2–3)

**Dependency:** VNMS factory import + `verify-enrollment` endpoint

- HTTPS VNMS client with CA pinning
- `POST /devices/register` end-to-end
- Device list/detail via VNMS `batchGet`
- Enable on successful enrollment

### Phase 4 — Monitored windows and timezone (week 3)

- Household and user timezone
- Local-time to UTC conversion for VNMS
- Delivery state from `device.policy_delivered` events

### Phase 5 — Event consumer and alerts (week 4)

- Poll VNMS `/v1/events` with cursor and idempotency
- Alert creation from monitored-window and status events
- Offline detection (`OFFLINE_THRESHOLD_HOURS`, default 3)
- GatewayAPI SMS for critical alert types

### Phase 6 — Push plumbing and trial logic (week 4–5)

- Push token registration
- Notification dispatcher (push stub + SMS + email)
- Trial expiry background job -> VNMS disable

### Phase 7 — UpCloud deploy and E2E (week 5+)

- Two VMs, private network, Caddy TLS
- Factory import script tested with sample batch
- E2E: import -> register -> enable -> movement -> alert

## Current Gaps (Doc vs Code)

Phases **1–5** are implemented. See [`implementation-status.md`](implementation-status.md).

| Area | Status |
|------|--------|
| Structure | Modular `internal/*` |
| Persistence | PostgreSQL (migrations `001`–`004`) |
| Auth | Argon2id + refresh rotation + MailerSend |
| Device claim | VNMS verify-enrollment + enable + Postgres bindings |
| Monitored windows | Local intent + UTC sync + `alert_mode` |
| Alerts | Postgres + VNMS event consumer + offline + SMS |
| Activity | Stub data (VNMS activity not wired) |
| Push tokens | In-memory (Phase 6) |
| Trial expiry job | Not started (Phase 6) |
| OpenAPI | Not created |
| Payments | Fake checkout URL |

## Open Items (Non-Blocking)

1. **QR payload format** — factory-generated; enrollment API accepts structured
   fields now and can add `qr_payload` parsing later.
2. **Payment provider** — decide before public launch; interface ready in code.
3. **Push provider** — decide with native app stack; UnifiedPush + APNs likely.
4. **VNMS disable reason in audit** — optional future field
   (`subscription_lapsed` vs `admin_action`).
5. **DST edge cases** — document limitations in UI for hourly monitored windows.

## Recommended Starting Order

1. VNMS: factory batch import as `disabled` + `verify-enrollment` endpoint
2. VSRV Phase 1: Docker + Postgres + refactor scaffold
3. Update [`vnms-integration.md`](vnms-integration.md) and
   [`device-lifecycle.md`](device-lifecycle.md) when VNMS enrollment API lands
4. `vigelo-frontend`: add `/invite/{token}` page

## Related Documents to Keep in Sync

When implementation progresses, update these if behavior diverges:

- [`vnms-integration.md`](vnms-integration.md) — enrollment verify flow
- [`device-lifecycle.md`](device-lifecycle.md) — factory import and register API
- [`mobile-api.md`](mobile-api.md) — `devices/register` replaces `device-claims`
- [`alerts-notifications.md`](alerts-notifications.md) — MailerSend, GatewayAPI
- [`subscriptions-payments.md`](subscriptions-payments.md) — trial-first MVP
- [`development-plan.md`](development-plan.md) — phase ordering
