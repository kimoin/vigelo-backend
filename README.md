# Vigelo Backend (VSRV)

Vigelo Backend, or VSRV, is the product backend for Vigelo. It serves the mobile
application, owns user accounts and household/device ownership, manages
subscriptions and payments, evaluates alert policy, sends notifications, and
integrates with Vigelo NMS (VNMS) for device-network actions.

VNMS owns the device plane: UDP, AEAD, replay counters, downlinks, monitored-window
delivery, device telemetry ingestion, and durable device facts. VSRV owns the
product plane: users, households, subscriptions, authorization, local-time user
intent, alert rules, mobile sessions, and user-facing APIs.

## Implementation status

**Phases 1–5 are implemented.** See [`docs/implementation-status.md`](docs/implementation-status.md)
for the full API list, database migrations, VNMS integration, alert behavior, and
local E2E steps.

| Phase | Summary |
|-------|---------|
| 1 | Go modules, Docker, Postgres, migrations |
| 2 | Auth, households, invites, MailerSend |
| 3 | VNMS enrollment, Postgres device bindings |
| 4 | Monitored windows, timezone, UTC → VNMS sync |
| 5 | Event consumer, Postgres alerts, offline detection, SMS |

## Design documents

- [`docs/implementation-status.md`](docs/implementation-status.md) — **what is built today**
- [`docs/vsrvplan.md`](docs/vsrvplan.md) — consolidated plan and decisions
- [`docs/architecture.md`](docs/architecture.md) — service boundaries
- [`docs/mobile-api.md`](docs/mobile-api.md) — mobile API direction
- [`docs/vnms-integration.md`](docs/vnms-integration.md) — VNMS integration
- [`docs/upcloud-deploy.md`](docs/upcloud-deploy.md) — **UpCloud: VSRV + VNMS two-VM setup**
- [`docs/alerts-notifications.md`](docs/alerts-notifications.md) — alerts and SMS
- [`docs/development-plan.md`](docs/development-plan.md) — phased plan

## Local development

### Prerequisites

- Go 1.26+
- Docker Desktop (Postgres)
- VNMS (`vigelo-nms`) for device enrollment

### Quick start

```sh
cp .env.example .env
make db-up
make migrate-docker
make run
```

Default listener: `http://127.0.0.1:8090`

**Admin console:** `http://127.0.0.1:8090/admin/` — sign in with an account listed in
`VSRV_ADMIN_EMAILS` (see `.env.example`).

`VSRV_DATABASE_URL` is required. Set `VNMS_BASE_URL` and `VNMS_HTTP_TOKEN` for
device enrollment. Without VNMS, auth and households work; device claim returns
service unavailable.

`GET /healthz` returns `{"status":"ok","database":"ok"}` when Postgres is reachable.

### With frontend

1. Start VSRV (`make run`).
2. Start `vigelo-frontend` (`npm run dev`).
3. Sign up, create a household.
4. Provision a device in VNMS (see implementation-status doc).
5. Claim with a real 32-char hex key: `device_id=860123456789012&key=...`
6. Set monitored hours and choose movement **or** no-movement alerts.

### Deploy layout

Production Compose (Postgres + migrate + vsrv + Caddy) lives under `deploy/`.
See [`deploy/README.md`](deploy/README.md) and [`docs/upcloud-deploy.md`](docs/upcloud-deploy.md)
for the full UpCloud + VNMS integration guide.

## Current limitations

- Activity API returns stub data (VNMS activity not wired yet).
- Push tokens are in Postgres; use `PUSH_PROVIDER=ntfy` for real delivery (or `apns` when native app ships).
- SMS logs only until `GATEWAYAPI_TOKEN` is set.
- Payments are demo checkout only; no payment provider.
- OpenAPI spec for VSRV not published yet.
