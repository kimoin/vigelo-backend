# VSRV Admin Console

The admin console is a built-in web UI served at **`/admin/`** on the VSRV HTTP
listener. It is intended for operators and support staff — not end users.

The UI is a single embedded HTML app (`internal/adminweb/web/index.html`) compiled
into the VSRV binary via `go:embed`. **Restart or redeploy VSRV** after UI changes
(local: `make run`; production: sync + `./scripts/update.sh`).

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

Click your **email** in the header to open a menu and **change your password**
(requires current password). Uses `POST /v1/auth/change-password` with your session
token (current session stays active).

Hard-refresh (`Cmd+Shift+R`) after server updates — admin assets use no-cache headers.

## UI overview

Modern light theme: soft gradient page background, frosted-glass header, white cards.
Accent palette uses **blue, cyan, yellow, and red** gradients (not a single mint/teal
highlight). Three tabs:

| Tab | Purpose | Active tab gradient |
|-----|---------|---------------------|
| **Status** | Host needle gauges + compact service health grid — **default tab on login** | Blue → cyan |
| **Manage** | Count widgets, user search, collapsible user/household/device tree | Cyan → indigo |
| **Audit log** | Searchable audit trail with pagination | Yellow → red |

Collapsible sections use a **▶** triangle that rotates downward when expanded.
User rows in the Manage table use the same triangle affordance.

### Progressive disclosure (add forms)

Inline “add” forms are **hidden by default**. Click the relevant **+ Add …** button
to reveal fields; **Cancel** hides them again without submitting.

| Location | Button | Revealed fields |
|----------|--------|-----------------|
| Households (user tree) | **+ Add household** | Name, optional timezone |
| Devices (per household) | **+ Add device** | Device ID, enrollment secret, display name, room |
| Co-members (per household) | **+ Add member** | Email, role, invite-if-new checkbox |

**+ Add user** (Manage toolbar) opens a modal — same pattern, no inline clutter.

### Removed devices

Under each household’s **Devices** section, removed bindings appear in a collapsible
**Removed devices (N)** list. Each row shows display name, device ID, binding ID, and
`removed_at` timestamp. The admin API includes `removed_at` on device detail JSON and
deduplicates bindings with `DISTINCT ON (b.id)` in the household device query.

## Status tab

Refreshes automatically every **8 seconds** while the tab is active.

### Host metrics (`GET /v1/admin/status` → `host`)

Needle gauges (0–100%) for:

| Gauge | Source |
|-------|--------|
| **CPU** | One widget per core (`host.cpus[]`) |
| **Memory** | Used % and bytes (`host.memory`) |
| **Storage** | Root filesystem used % (`host.storage`) |
| **Network** | Throughput gauge with rx/tx bytes/sec (`host.network`) |

On Linux production, Docker mounts host `/proc` at `/host/proc` and sets
`VSRV_PROC_ROOT=/host/proc` so gauges reflect the VM, not only the container.
On macOS dev builds, host metrics may be empty.

### Service health (`host` + `services`)

Compact grid below the gauges. Long status detail text wraps within each card.

| Service | Check |
|---------|-------|
| **Database** | Postgres ping |
| **VNMS** | `GET /healthz` with bearer token |
| **MailerSend** | API token + from-domain/sender-identity verification |
| **GatewayAPI** | `GET /rest/me` with token |
| **UnifiedPush / ntfy** | `GET /v1/health` when `PUSH_PROVIDER=ntfy` |
| **APNs** | Credential presence only (stub until native app) |

Status values: `ok`, `fail`, `down`, `log_only`, `unconfigured`, `not_active`, `stub`.

## Manage tab

### Count widgets (top row)

Summary counts from `GET /v1/admin/dashboard`: users, households, devices,
trialing subscriptions, active alerts. Each widget has a colour-coded gradient top
bar (blue, cyan, yellow, red cycling) and a matching number colour.

### Search and pagination

| Control | Behaviour |
|---------|-----------|
| **Search** | Text query + filter (resets to page 1) |
| **Per page** | 25 (default) or 100 |
| **Previous / Next** | Offset pagination via `limit` + `offset` |

| Filter | Finds |
|--------|--------|
| Email / name | User email, display name, or user id |
| Device ID | Users linked to a device binding |
| Expired trial | Trialing subscriptions past `trial_ends_at` |
| Failed payment | `past_due` or `service_suspended` |

### Add user (`+ Add user` modal)

Creates an account immediately — **no invitation email**; `email_verified_at` is set
on creation. Fields: email, password (min 8), display name, account type
(`user` | `admin`).

- **End user** — mobile app account with default "Home" household.
- **Admin** — same DB account; add email to `VSRV_ADMIN_EMAILS` and redeploy for
  console access.

`POST /v1/admin/users` body:

```json
{
  "email": "user@example.com",
  "password": "secret123",
  "display_name": "Optional",
  "account_type": "user"
}
```

### User list and collapsible tree

Click a user row (▶ triangle) to expand. Tree sections:

| Section | Actions |
|---------|---------|
| **User** | Disable, enable, delete (if allowed) |
| **Households** | **+ Add household** (inline form on demand), − archive/remove; nested devices |
| **Devices** | **+ Add device** (inline form on demand), unprovision, remove; time windows; last seen & battery |
| **Co-members** | List other members with household + role; **+ Add member** per household (form on demand) |
| **Payments** | Subscription status; extend trial (custom days); mark paid |

**Delete user** (`DELETE /v1/admin/users/{user_id}`):

- Cannot delete your own account while signed in.
- Cannot delete emails listed in `VSRV_ADMIN_EMAILS` (protected `.env` admins).
- Cleans up owned households, sessions, and invites.

User detail enriches device rows from VNMS (battery voltage, last seen, online/offline).

### Time management (monitored windows)

Per device in the tree. Loads and saves via admin routes (same logic as mobile):

```
GET /v1/admin/devices/{device_binding_id}/monitored-windows
PUT /v1/admin/devices/{device_binding_id}/monitored-windows
```

Body for PUT: `{ "windows": [{ "start_time": "08:00", "end_time": "20:00" }], "alert_mode": "no_movement_detected" }`

### Member invite states

| Badge | Meaning |
|-------|---------|
| **Invited** (yellow gradient) | Pending invite; shows invitation time and expiry |
| **Registered** (cyan gradient) | User joined the household |
| **Failed** (red gradient) | Invite expired without acceptance |

Use **invite if new** when adding a member by email — creates invite if user does not exist.

### Device actions (API reference)

| Action | API | Notes |
|--------|-----|-------|
| Enable (VNMS) | `POST /v1/admin/devices/{id}/enable` | Calls VNMS enable |
| Disable (VNMS) | `POST /v1/admin/devices/{id}/disable` | Calls VNMS disable |
| Unprovision | `POST /v1/admin/devices/{id}/unprovision` | VNMS unprovision |
| Extend trial | `POST .../extend-trial` | Body or query `{ "days": N }` |
| Mark paid | `POST .../activate-subscription` | Demo: sets active for 1 month |
| Move device | `POST .../move` | Target `household_id` |
| Delete device | `DELETE .../devices/{id}` | Soft-remove binding |
| Provision device | `POST /v1/admin/households/{hh}/devices` | `device_id` + 32-char hex key; VSRV provisions NMS on first use |

**Test device cleanup (VNMS):** unprovision then `DELETE /v1/devices/{device_id}` on
VNMS (requires unprovisioned state). Remove the VSRV binding separately via
**Delete device** above.

## Audit log tab

Search with `q`. Pagination: **25** default, selectable **100** (`limit`, `offset`).

Requires migration `005_audit_message.sql` (applied automatically on startup).

Retention: `VSRV_AUDIT_RETENTION_DAYS` (default 60). A background job prunes older entries.

Logged actions include: signup, login, password change, user create/delete, device
provision, admin user/household/device changes, trial extensions, monitored-window
updates, notifications sent, trial expiry.

## Admin HTTP API

All routes require `Authorization: Bearer <access_token>` and admin email.

```
GET  /v1/admin/me
GET  /v1/admin/status              # services[] + host metrics
GET  /v1/admin/dashboard           # count widgets
GET  /v1/admin/audit-log?q=&limit=&offset=

GET  /v1/admin/users?q=&filter=&limit=&offset=
GET  /v1/admin/users/{user_id}
POST /v1/admin/users
DELETE /v1/admin/users/{user_id}
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
GET  /v1/admin/devices/{device_binding_id}/monitored-windows
PUT  /v1/admin/devices/{device_binding_id}/monitored-windows
```

User list **`filter`** query values: `email` (default), `device_id`, `expired_trial`,
`failed_payment`.

Account password (any authenticated user, including admins in the console UI):

```
POST /v1/auth/change-password   # { "current_password", "new_password" }
```

## Troubleshooting

| Symptom | Cause / fix |
|---------|-------------|
| UI changes not visible after edit | Admin HTML is embedded — rebuild/restart VSRV |
| Audit log shows "audit log failed" | Run migrations; ensure `005_audit_message.sql` applied |
| No trial/payment data | No devices registered — provision a device first |
| Enable user button fails | Fixed: admin enable must not use `GetUserByID` on disabled users |
| MailerSend shows `configured` but emails fail | Status now checks domain verification live |
| VNMS shows `unconfigured` | Set `VNMS_BASE_URL` and `VNMS_HTTP_TOKEN` |
| Device provision fails | VNMS must be reachable; enrollment key must be valid |
| Host CPU gauges all 0% on first load | Normal — second refresh computes delta from `/proc/stat` |
| Cannot delete admin user | Email is in `VSRV_ADMIN_EMAILS` — remove from env first |
