# VSRV Admin Console

The admin console is a built-in web UI served at **`/admin/`** on the VSRV HTTP
listener. It is intended for operators and support staff — not end users.

## Access control

There is **no separate admin password**. Access works as follows:

1. User signs in via the normal API: `POST /v1/auth/login`
2. VSRV checks whether the user's email is listed in `VSRV_ADMIN_EMAILS`
3. If yes, admin API routes and the `/admin/` UI are allowed

Configure in `.env`:

```env
VSRV_ADMIN_EMAILS=ops@example.com,support@example.com
```

Local URL: `http://127.0.0.1:8090/admin/`  
Production URL: `https://<VSRV_HOSTNAME>/admin/`

## UI tabs

| Tab | Purpose |
|-----|---------|
| **Users** | Search users; expand for households, devices, subscriptions, invites |
| **Status** | Live health of Database, VNMS, MailerSend, GatewayAPI, push providers |
| **Audit log** | Searchable audit trail of admin and system actions |
| **Dashboard** | Counts: users, households, devices, subscriptions |

Hard-refresh (`Cmd+Shift+R`) after server updates — admin assets use no-cache headers.

## Users tab — drill-down

Click a user row to expand:

- Account status (active/disabled), email verification, sessions, push tokens
- **Trial & payment status** — table of all devices with subscription state
- **Households** — members, invites, devices

### Member invite states

| Badge | Meaning |
|-------|---------|
| **Invited** (yellow) | Pending invite; shows invitation time and expiry |
| **Registered** (green) | User joined the household |
| **Failed** (red) | Invite expired without acceptance |

### Device actions (per device)

| Action | API | Notes |
|--------|-----|-------|
| Enable (VNMS) | `POST /v1/admin/devices/{id}/enable` | Calls VNMS enable |
| Disable (VNMS) | `POST /v1/admin/devices/{id}/disable` | Calls VNMS disable |
| Unprovision | `POST /v1/admin/devices/{id}/unprovision` | VNMS unprovision |
| Extend trial (+7d / +30d / custom) | `POST .../extend-trial` | Extends from current trial end |
| Mark paid | `POST .../activate-subscription` | Demo: sets active for 1 month |
| Move device | `POST .../move` | Dropdown of user's other households |
| Delete device | `DELETE .../devices/{id}` | Soft-remove binding |
| Provision device | `POST /v1/admin/households/{hh}/devices` | Same enrollment as mobile: `device_id` + 32-char hex key; VSRV provisions NMS on first use |

**Test device cleanup (VNMS):** unprovision then `DELETE /v1/devices/{device_id}` on
VNMS (requires unprovisioned state). Remove the VSRV binding separately via
**Delete device** above.

### Household actions

| Action | API |
|--------|-----|
| Add member | `POST /v1/admin/households/{id}/members` |
| Remove member | `DELETE .../members/{user_id}` |
| Archive household | `DELETE /v1/admin/households/{id}` |
| New household | `POST /v1/admin/households` |

Use **invite if new** when adding a member by email — creates invite if user does not exist.

## Status tab — live health checks

The Status tab calls external services on each load (not just config checks):

| Service | Check |
|---------|-------|
| **Database** | Postgres ping |
| **VNMS** | `GET /healthz` with bearer token |
| **MailerSend** | API token + from-domain/sender-identity verification |
| **GatewayAPI** | `GET /rest/me` with token |
| **UnifiedPush / ntfy** | `GET /v1/health` when `PUSH_PROVIDER=ntfy` |
| **APNs** | Credential presence only (stub until native app) |

Status values: `ok`, `fail`, `down`, `log_only`, `unconfigured`, `not_active`, `stub`.

## Audit log

Requires migration `005_audit_message.sql` (applied automatically on startup).

Retention: `VSRV_AUDIT_RETENTION_DAYS` (default 60). A background job prunes older entries.

Logged actions include: signup, login, device provision, admin user/household/device
changes, trial extensions, notifications sent, trial expiry.

## Admin HTTP API

All routes require `Authorization: Bearer <access_token>` and admin email.

```
GET  /v1/admin/me
GET  /v1/admin/status
GET  /v1/admin/dashboard
GET  /v1/admin/audit-log?q=&limit=&offset=

GET  /v1/admin/users?q=&limit=&offset=
GET  /v1/admin/users/{user_id}
POST /v1/admin/users/{user_id}/disable
POST /v1/admin/users/{user_id}/enable

GET  /v1/admin/households
POST /v1/admin/households
DELETE /v1/admin/households/{household_id}
POST /v1/admin/households/{household_id}/members
DELETE /v1/admin/households/{household_id}/members/{user_id}
POST /v1/admin/households/{household_id}/devices

GET  /v1/admin/devices
POST /v1/admin/devices/{device_binding_id}/enable
POST /v1/admin/devices/{device_binding_id}/disable
POST /v1/admin/devices/{device_binding_id}/unprovision
DELETE /v1/admin/devices/{device_binding_id}
POST /v1/admin/devices/{device_binding_id}/move
POST /v1/admin/devices/{device_binding_id}/extend-trial
POST /v1/admin/devices/{device_binding_id}/activate-subscription
```

## Troubleshooting

| Symptom | Cause / fix |
|---------|-------------|
| Audit log shows "audit log failed" | Run migrations; ensure `005_audit_message.sql` applied |
| No trial/payment data | No devices registered — provision a device first |
| Enable user button fails | Fixed: admin enable must not use `GetUserByID` on disabled users |
| MailerSend shows `configured` but emails fail | Status now checks domain verification live |
| VNMS shows `unconfigured` | Set `VNMS_BASE_URL` and `VNMS_HTTP_TOKEN` |
| Device provision fails | VNMS must be reachable; device must exist in VNMS inventory |
