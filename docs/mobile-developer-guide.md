# Vigelo Mobile API — Developer Guide

**Audience:** iOS and Android engineers integrating with VSRV (Vigelo Backend).  
**Status:** Reflects the **implemented** API as of 2026-07-10.  
**Design direction:** [`mobile-api.md`](mobile-api.md) (future endpoints and OpenAPI).  
**Implementation truth:** [`implementation-status.md`](implementation-status.md).

---

## 1. What VSRV Is

VSRV is the **only backend** the Vigelo mobile app talks to. It owns:

- User accounts, sessions, and household membership
- Device bindings (product view of a physical Vigelo sensor)
- Monitored-window schedules and alert policy
- Subscriptions and trial state
- Push notification delivery

VSRV integrates with **VNMS** (Vigelo Network Management Server) behind the scenes. The mobile app must **not** call VNMS directly and does not need to know about UDP, NB-IoT, device keys after claim, or raw network telemetry formats.

```text
Mobile app  --HTTPS JSON-->  VSRV (/v1/*)
                                |
                                +--> PostgreSQL (users, devices, alerts)
                                +--> VNMS (device enrollment, policy, telemetry)
                                +--> MailerSend (email)
                                +--> GatewayAPI (SMS)
                                +--> APNs / FCM / ntfy (push)
```

---

## 2. Transport and Conventions

| Topic | Rule |
|-------|------|
| **Base URL** | Production: `VSRV_PUBLIC_URL` (e.g. `https://api.vigelo.fi`). Local dev: `http://127.0.0.1:8090` |
| **API prefix** | `/v1` |
| **Protocol** | HTTPS in production; JSON request and response bodies |
| **Content-Type** | `application/json` |
| **Timestamps** | UTC, RFC3339 (e.g. `2026-07-10T14:30:00Z`) |
| **Resource IDs** | Opaque strings with prefixes: `user_…`, `hh_…`, `devbind_…`, `alert_…`, `push_…`, `inv_…` |
| **Real-time** | No WebSocket or SSE. Use push notifications + foreground refresh. |

### CORS

Relevant for web prototypes only. Allowed origins are configured via `VSRV_CORS_ORIGIN`. Preflight `OPTIONS` returns `204`.

### Rate limiting

**Not implemented.** Do not rely on `429` responses today.

---

## 3. Authentication

### 3.1 Bearer tokens

All protected endpoints require:

```http
Authorization: Bearer <access_token>
```

- Tokens are **opaque strings**, not JWTs.
- Store `access_token` and `refresh_token` in **iOS Keychain** or **Android Keystore**.
- Cookies are not used.

### 3.2 Token lifetimes (server defaults)

| Token | Default TTL | Env var |
|-------|-------------|---------|
| Access token | 1 hour | `VSRV_ACCESS_TOKEN_TTL_HOURS` |
| Refresh token | 30 days | `VSRV_REFRESH_TOKEN_TTL_DAYS` |

### 3.3 Session lifecycle

```text
signup/login
  -> receive access_token + refresh_token
  -> store both securely

before access_token expires (or on 401):
  POST /v1/auth/refresh  (Authorization: Bearer <access_token>)
  -> receive NEW access_token + refresh_token (rotation)
  -> replace stored tokens

logout:
  POST /v1/auth/logout   (Authorization: Bearer <access_token>)
  -> revoke session server-side
  -> delete local tokens
```

**Important implementation detail:** `POST /v1/auth/refresh` authenticates with the **current access token** in the `Authorization` header. It does **not** accept `refresh_token` in the request body. The refresh token is returned for forward compatibility and secure storage, but rotation is driven by the access token today.

**Password reset** (`POST /v1/auth/password-reset/complete`) revokes **all** sessions for that user. The app must clear local tokens and return to login.

### 3.4 Recommended client token manager

```text
on API 401 unauthorized:
  if refresh not yet attempted for this request:
    call POST /v1/auth/refresh
    if success: retry original request with new access_token
    if failure: clear tokens, navigate to login
  else:
    clear tokens, navigate to login

proactive refresh:
  schedule refresh ~5 minutes before access_token expiry while app is active
```

---

## 4. Error Envelope

Every error response uses this shape:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "Human readable message",
    "field": "email"
  }
}
```

`field` is omitted when not applicable (validation field name for form binding).

### Error codes in use

| Code | HTTP | When |
|------|------|------|
| `invalid_request` | 400 | Bad JSON, missing fields, validation failure |
| `unauthorized` | 401 | Missing/invalid/expired token, bad login |
| `forbidden` | 403 | Authenticated but role or enrollment denied |
| `not_found` | 404 | Resource missing |
| `conflict` | 409 | Duplicate claim, already member, etc. |
| `service_unavailable` | 503 | Database down, VNMS not configured |
| `internal_error` | 500 | Unexpected server error |

### Codes documented but not yet returned

`rate_limited`, `subscription_required`, `device_not_ready`.

---

## 5. Household Roles and Permissions

Each household member has one role. The caller's role is returned on `Household` objects as `role`.

| Role | View devices | Claim/remove devices | Configure devices & monitored windows | Manage household & invites |
|------|:------------:|:--------------------:|:-------------------------------------:|:--------------------------:|
| `owner` | ✓ | ✓ | ✓ | ✓ |
| `admin` | ✓ | ✓ | ✓ | ✓ |
| `caregiver` | ✓ | ✗ | ✗ | ✗ |
| `member` | ✓ | ✗ | ✗ | ✗ |

Billing management (`CanManageBilling`) is owner-only in code but no dedicated billing-role checks exist on checkout yet.

---

## 6. Shared Data Types

### 6.1 User (public)

Returned from auth and profile endpoints. Password hash and phone are never exposed in these responses.

```json
{
  "id": "user_abc123",
  "email": "user@example.com",
  "display_name": "Kimmo",
  "created_at": "2026-07-01T10:00:00Z",
  "email_verified_at": "2026-07-01T11:00:00Z",
  "timezone": "Europe/Helsinki"
}
```

`email_verified_at` and `timezone` are omitted when unset.

### 6.2 Household

```json
{
  "id": "hh_xyz789",
  "name": "Home",
  "timezone": "Europe/Helsinki",
  "country": "FI",
  "role": "owner",
  "created_at": "2026-07-01T10:00:00Z"
}
```

`country` and `role` may be omitted depending on context. List responses always include `role` (caller's role in that household).

### 6.3 HouseholdMember

```json
{
  "user_id": "user_def456",
  "email": "caregiver@example.com",
  "display_name": "Anna",
  "role": "caregiver",
  "created_at": "2026-07-08T09:00:00Z"
}
```

### 6.4 DeviceBinding

The primary device object for list and detail screens.

```json
{
  "id": "devbind_001",
  "household_id": "hh_xyz789",
  "device_id": "867530012345678",
  "display_name": "Living room",
  "room_or_location_label": "Living room",
  "status": "online",
  "last_seen_at": "2026-07-10T12:00:00Z",
  "battery_voltage_v": 3.05,
  "battery_status": "ok",
  "subscription_status": "trialing",
  "monitored_windows": [
    { "start_time": "08:00", "end_time": "20:00" }
  ],
  "monitored_windows_delivery_state": "delivered",
  "monitored_window_alert_mode": "no_movement_detected",
  "active_alert_count": 1,
  "subscription": {
    "status": "trialing",
    "service_status": "service_active",
    "plan_code": "device_monitoring_monthly",
    "current_period_end": "2026-08-09T10:00:00Z",
    "next_action": null
  },
  "created_at": "2026-07-05T08:00:00Z",
  "updated_at": "2026-07-10T12:00:00Z"
}
```

#### Device `status` values

| Value | Meaning |
|-------|---------|
| `waiting_for_first_contact` | Claimed but device has not reported to network yet |
| `online` | Last contact within offline threshold (default 3 hours) |
| `offline` | No contact beyond offline threshold |
| `service_suspended` | VNMS lifecycle is `disabled` (admin action or subscription lapse) |

#### `battery_status` values

| Value | Voltage (approx.) |
|-------|-------------------|
| `unknown` | No reading |
| `ok` | ≥ 2.9 V |
| `low` | 2.7 – 2.9 V |
| `critical` | < 2.7 V |

#### `monitored_windows_delivery_state` values

| Value | Meaning |
|-------|---------|
| `not_configured` | No windows saved yet |
| `pending_delivery` | Saved locally; awaiting device confirmation |
| `sync_failed` | VNMS call failed or VNMS not configured |
| `delivered` | Device policy matches saved intent |

#### `monitored_window_alert_mode` values (mutually exclusive)

| Value | Alert when |
|-------|------------|
| `no_movement_detected` | No movement during monitored window (**default**) |
| `movement_detected` | Movement detected during monitored window |

`device_offline` alerts are independent of this mode.

### 6.5 MonitoredWindow

Whole-hour times in 24-hour `HH:00` format:

```json
{ "start_time": "08:00", "end_time": "20:00" }
```

Rules enforced by API:

- Up to **2** non-overlapping windows
- Whole hours only (`08:00`, not `08:30`)
- A full-day window cannot be combined with another
- Windows repeat **daily** (no per-weekday schedule in v1)

### 6.6 Subscription

```json
{
  "status": "trialing",
  "service_status": "service_active",
  "plan_code": "device_monitoring_monthly",
  "current_period_end": "2026-08-09T10:00:00Z",
  "next_action": null
}
```

| `status` | Meaning |
|----------|---------|
| `trialing` | Free trial; `current_period_end` is trial end |
| `active` | Paid period active |
| `past_due` | Trial expired or payment failed; service may be suspended |

| `service_status` | Meaning |
|------------------|---------|
| `service_active` | Monitoring and alerts enabled |
| `service_suspended` | Service paused (trial end, admin, etc.) |

`next_action` exists in the schema but is **always `null`** today. Derive UI copy from `status` + `current_period_end`.

### 6.7 Alert

```json
{
  "id": "alert_001",
  "device_binding_id": "devbind_001",
  "type": "no_movement_detected",
  "severity": "warning",
  "status": "active",
  "title": "No movement detected",
  "body": "Living room — no movement during the monitored window.",
  "first_seen_at": "2026-07-10T08:05:00Z",
  "last_seen_at": "2026-07-10T08:05:00Z",
  "acknowledged_at": null
}
```

| `type` | `severity` (typical) |
|--------|----------------------|
| `movement_detected` | `info` |
| `no_movement_detected` | `warning` |
| `device_offline` | `critical` |

| `status` | Meaning |
|----------|---------|
| `active` | Open alert |
| `acknowledged` | User tapped acknowledge |
| `resolved` | Cleared server-side |

### 6.8 PushToken (registration response)

The raw push token is **never** returned after registration.

```json
{
  "id": "push_001",
  "platform": "ios",
  "environment": "production",
  "token_hint": "abcd...wxyz",
  "created_at": "2026-07-10T10:00:00Z"
}
```

| `platform` | Accepted values |
|------------|-----------------|
| | `ios`, `android`, `web`, `ntfy`, `unifiedpush` |

---

## 7. Endpoint Reference

Unless noted, all paths are under the API base URL. **Auth** column: `—` = public, `Bearer` = access token required.

### 7.1 Health

#### `GET /healthz` — Auth: —

Liveness probe. Does not require authentication.

**200 OK**

```json
{ "status": "ok", "database": "ok" }
```

**503 Service Unavailable** (database unreachable)

```json
{ "status": "degraded", "database": "unavailable" }
```

---

### 7.2 Authentication and Profile

#### `POST /v1/auth/signup` — Auth: —

Create account. Auto-creates a household named **"Home"** and logs the user in.

**Request**

```json
{
  "email": "user@example.com",
  "password": "minimum8chars",
  "display_name": "Kimmo"
}
```

| Field | Required | Notes |
|-------|----------|-------|
| `email` | yes | Lowercased and trimmed server-side |
| `password` | yes | Minimum 8 characters |
| `display_name` | no | Defaults to email |

**201 Created**

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "user": { },
  "household": { }
}
```

Side effect: verification email sent to `{FRONTEND_BASE_URL}/verify-email?token=...`.

**Errors:** `400` (`email`, `password`); `409` email taken; `503` database unavailable.

---

#### `POST /v1/auth/login` — Auth: —

**Request**

```json
{
  "email": "user@example.com",
  "password": "..."
}
```

**200 OK**

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "user": { }
}
```

**Errors:** `401` invalid credentials or disabled account.

---

#### `POST /v1/auth/refresh` — Auth: Bearer

Rotate session. **No request body.**

**200 OK**

```json
{
  "access_token": "...",
  "refresh_token": "..."
}
```

**Errors:** `401` invalid or expired session.

---

#### `POST /v1/auth/logout` — Auth: Bearer

Revoke current session.

**200 OK**

```json
{ "status": "logged_out" }
```

---

#### `POST /v1/auth/verify-email` — Auth: —

**Request**

```json
{ "token": "raw-token-from-email-link" }
```

**200 OK**

```json
{ "status": "verified" }
```

**Errors:** `400` invalid or expired token.

The mobile app typically handles this via a deep link that extracts `token` from the URL query string.

---

#### `POST /v1/auth/password-reset/request` — Auth: —

**Request**

```json
{ "email": "user@example.com" }
```

**200 OK** (always, even if email unknown — prevents enumeration)

```json
{ "status": "ok" }
```

Email link format: `{FRONTEND_BASE_URL}/reset-password?token=...`

---

#### `POST /v1/auth/password-reset/complete` — Auth: —

**Request**

```json
{
  "token": "...",
  "new_password": "minimum8chars"
}
```

**200 OK**

```json
{ "status": "password_updated" }
```

Revokes all sessions. App must force re-login.

---

#### `GET /v1/me` — Auth: Bearer

**200 OK**

```json
{ "user": { } }
```

---

#### `PATCH /v1/me` — Auth: Bearer

Partial update. Omit fields to leave unchanged; send `null` to skip (pointer semantics).

**Request**

```json
{
  "display_name": "New Name",
  "timezone": "Europe/Helsinki"
}
```

**200 OK**

```json
{ "user": { } }
```

Use IANA timezone identifiers (e.g. `Europe/Helsinki`, `America/New_York`).

---

### 7.3 Households and Invites

#### `GET /v1/households` — Auth: Bearer

List households the user belongs to.

**200 OK**

```json
{ "households": [ { } ] }
```

---

#### `POST /v1/households` — Auth: Bearer

**Request**

```json
{
  "name": "Summer cabin",
  "timezone": "Europe/Helsinki"
}
```

Defaults: `name` → `"Home"`, `timezone` → `"Europe/Helsinki"`.

**201 Created** — `Household` object (caller is `owner`).

---

#### `PATCH /v1/households/{household_id}` — Auth: Bearer (owner/admin)

**Request**

```json
{
  "name": "Updated name",
  "timezone": "Europe/Helsinki"
}
```

**200 OK** — updated `Household`.

**Errors:** `403` if role is `caregiver` or `member`.

---

#### `GET /v1/households/{household_id}/members` — Auth: Bearer (any member)

**200 OK**

```json
{ "members": [ { } ] }
```

---

#### `POST /v1/households/{household_id}/invites` — Auth: Bearer (owner/admin)

**Request**

```json
{
  "email": "caregiver@example.com",
  "role": "caregiver"
}
```

| Field | Default | Allowed roles |
|-------|---------|---------------|
| `role` | `caregiver` | `admin`, `caregiver`, `member` — **not** `owner` |

**201 Created** — `HouseholdInvite` (raw invite token is **not** returned; sent by email).

```json
{
  "id": "inv_001",
  "household_id": "hh_xyz789",
  "email": "caregiver@example.com",
  "role": "caregiver",
  "expires_at": "2026-07-17T10:00:00Z",
  "created_at": "2026-07-10T10:00:00Z"
}
```

Email link: `{FRONTEND_BASE_URL}/invite/{token}`

---

#### `POST /v1/invites/{token}/accept` — Auth: Bearer

Accept invite. User must be logged in; invite email must match account email.

**Request body:** none.

**200 OK**

```json
{ "household": { } }
```

**Errors:** `404` invite not found/expired/wrong email; `409` already a member.

Deep link flow: app opens `/invite/{token}`, user signs in if needed, then calls this endpoint.

---

### 7.4 Push Tokens

Register after obtaining APNs/FCM device token. Re-register on token refresh and after login.

#### `POST /v1/push-tokens` — Auth: Bearer

**Request**

```json
{
  "platform": "ios",
  "token": "apns-device-token-hex",
  "environment": "production"
}
```

| Field | Required | Notes |
|-------|----------|-------|
| `platform` | yes | `ios`, `android`, `web`, `ntfy`, `unifiedpush` |
| `token` | yes | Raw provider token |
| `environment` | no | Defaults to `production`; use `sandbox` for APNs dev builds |

**201 Created** — `PushToken` (see §6.8).

Upsert semantics: same user + platform + token updates the existing row.

**Errors:** `400` invalid platform or missing token.

---

#### `DELETE /v1/push-tokens/{push_token_id}` — Auth: Bearer

Unregister token (call on logout).

**200 OK**

```json
{ "status": "deleted" }
```

Idempotent — returns 200 even if ID not found.

---

### 7.5 Device Enrollment

Two endpoints perform the same enrollment logic. Use **`device-claims`** for QR scan flow; use **`devices/register`** for manual entry.

#### `POST /v1/households/{household_id}/device-claims` — Auth: Bearer (owner/admin)

QR-oriented. Parses `qr_payload` for `device_id` / `imei` and `key` / `enrollment_secret`.

#### `POST /v1/households/{household_id}/devices/register` — Auth: Bearer (owner/admin)

Manual entry. Uses explicit `device_id` + `enrollment_secret`.

**Request**

```json
{
  "qr_payload": "device_id=867530012345678&key=abc123...",
  "device_id": "867530012345678",
  "enrollment_secret": "abc123def456",
  "display_name": "Living room",
  "room_or_location_label": "Living room",
  "timezone": "Europe/Helsinki"
}
```

| Field | Notes |
|-------|-------|
| `qr_payload` | Full scanned string. Separators: `&`, `?`, `;`, `,`, newline |
| `device_id` | IMEI or factory device ID |
| `enrollment_secret` | Hex key; spaces stripped |
| `display_name` | Default `"Vigelo device"` |
| `room_or_location_label` | Default `"Home"` |
| `timezone` | Default household timezone |

**QR parsing** (claim endpoint): extracts from payload:

- Device ID: `device_id=`, `imei=`, or bare 8–64 char string
- Secret: `key=`, `device_key=`, `device_key_hex=`, `enrollment_secret=`

**201 Created** — full `DeviceBinding`.

**Server-side enrollment steps**

1. Verify caller is owner/admin of household
2. Validate `device_id` and 32-character hex `enrollment_secret`
3. Call VNMS `verify-enrollment`; if the device is new or unprovisioned in NMS, call `provision-inventory`
4. Create `device_bindings` row + `trialing` subscription (default 30 days)
5. Call VNMS `enable`
6. Merge live VNMS state into response

**Errors**

| HTTP | Code | Field | Condition |
|------|------|-------|-----------|
| 400 | `invalid_request` | `enrollment_secret` | Missing fields or invalid key format (must be 32 hex chars) |
| 403 | `forbidden` | `enrollment_secret` | Key rejected by VNMS |
| 403 | `forbidden` | — | Role not owner/admin |
| 409 | `conflict` | `device_id` | Already claimed or active elsewhere |
| 503 | `service_unavailable` | — | VNMS not configured |

**Security:** The enrollment secret is used once at claim time. It is **never** returned in API responses afterward.

---

#### `GET /v1/households/{household_id}/devices` — Auth: Bearer (any member)

**200 OK**

```json
{ "devices": [ { } ] }
```

Sorted by `created_at` ascending. Each device includes `active_alert_count`.

---

#### `GET /v1/devices/{device_binding_id}` — Auth: Bearer (any member)

**200 OK** — full `DeviceBinding`.

**Errors:** `404` not found; `403` not a household member.

---

#### `PATCH /v1/devices/{device_binding_id}` — Auth: Bearer (owner/admin)

**Request**

```json
{
  "display_name": "Bedroom",
  "room_or_location_label": "Upstairs"
}
```

Only non-empty strings update fields.

**200 OK** — updated `DeviceBinding`.

---

#### `POST /v1/devices/{device_binding_id}/remove` — Auth: Bearer (owner/admin)

Soft-delete binding (`removed_at` set). Does not factory-reset the physical device.

**200 OK**

```json
{ "status": "removed" }
```

---

### 7.6 Monitored Windows

#### `GET /v1/devices/{device_binding_id}/monitored-windows` — Auth: Bearer (any member)

**200 OK**

```json
{
  "timezone": "Europe/Helsinki",
  "windows": [
    { "start_time": "08:00", "end_time": "20:00" }
  ],
  "alert_mode": "no_movement_detected",
  "delivery_state": "delivered"
}
```

---

#### `PUT /v1/devices/{device_binding_id}/monitored-windows` — Auth: Bearer (owner/admin)

Replaces the full schedule. Send empty `windows` array to clear.

**Request**

```json
{
  "timezone": "Europe/Helsinki",
  "windows": [
    { "start_time": "08:00", "end_time": "20:00" }
  ],
  "alert_mode": "no_movement_detected"
}
```

| Field | Default |
|-------|---------|
| `timezone` | Device/household timezone |
| `alert_mode` | `no_movement_detected` |

**200 OK** — same shape as GET.

**Side effects**

1. Validates windows (max 2, no overlap, whole hours)
2. Converts local time → UTC for VNMS
3. Calls VNMS `SetMonitoredWindows`
4. Enables selected movement alert rule; disables the other
5. Sets `delivery_state` to `pending_delivery`, `sync_failed`, or `delivered`

**Errors:** `400` with `field: "windows"` and validation message.

---

### 7.7 Activity (stub)

#### `GET /v1/devices/{device_binding_id}/activity` — Auth: Bearer (any member)

**⚠️ Returns synthetic demo data**, not real telemetry. Do not ship production UI that depends on this endpoint.

Query params `start` and `end` documented in design are **ignored**.

**200 OK**

```json
{
  "timezone": "Europe/Helsinki",
  "days": [
    {
      "date": "2026-07-10",
      "hours": [
        {
          "start": "08:00",
          "end": "09:00",
          "movement": true,
          "monitored": true,
          "event": "movement_detected"
        }
      ]
    }
  ]
}
```

Returns last 7 days, 24 hours per day.

---

### 7.8 Alerts

#### `GET /v1/devices/{device_binding_id}/alerts` — Auth: Bearer (any member)

**200 OK**

```json
{ "alerts": [ { } ] }
```

Sorted by `first_seen_at` descending (newest first).

---

#### `POST /v1/devices/{device_binding_id}/alerts/{alert_id}/ack` — Auth: Bearer (any member)

No request body.

**200 OK** — updated `Alert` with `status: "acknowledged"` and `acknowledged_at` set.

**Errors:** `404` alert not found for this device.

---

### 7.9 Subscriptions

#### `GET /v1/devices/{device_binding_id}/subscription` — Auth: Bearer (any member)

**200 OK** — `Subscription` object (subset of device detail).

---

#### `POST /v1/devices/{device_binding_id}/subscription/checkout` — Auth: Bearer (any member)

**⚠️ Demo stub** — no real payment provider. Immediately activates subscription for 1 month.

No request body.

**200 OK**

```json
{
  "status": "active",
  "checkout_url": "dev://checkout/succeeded",
  "subscription": { }
}
```

Production will return a real `checkout_url` (Stripe or similar). Mobile should open checkout URL in SFSafariViewController / Chrome Custom Tab when integrated.

---

## 8. App Integration Flows

### 8.1 Cold start / session restore

```text
1. Load access_token + refresh_token from secure storage
2. If no tokens -> show login/signup
3. GET /v1/me
   - 200 -> main app (optionally GET /v1/households)
   - 401 -> POST /v1/auth/refresh -> retry /v1/me or show login
4. POST /v1/push-tokens if platform token available
```

### 8.2 Onboarding (new user)

```text
1. POST /v1/auth/signup
2. Store tokens; user + household returned
3. Register push token
4. Navigate to device claim or home (household "Home" already exists)
5. Prompt user to verify email (optional banner; app works before verify)
```

### 8.3 Device claim (QR)

```text
1. User selects household (or uses default from signup)
2. Scan QR -> obtain qr_payload string
3. POST /v1/households/{id}/device-claims
   { "qr_payload": "...", "display_name": "...", "room_or_location_label": "..." }
4. On 201 -> show device detail; status may be waiting_for_first_contact
5. PUT monitored-windows if user configures schedule
6. If subscription.status == trialing -> show trial end date; prompt checkout before expiry
```

### 8.4 Invite acceptance

```text
1. User opens email link -> app deep link /invite/{token}
2. If not logged in -> login/signup first (email must match invite)
3. POST /v1/invites/{token}/accept
4. Navigate to household device list (caregiver role: view only)
```

### 8.5 Alert handling

```text
Push received (title/body from server)
  -> user taps notification
  -> deep link to device alerts screen
  -> GET /v1/devices/{id}/alerts
  -> POST .../alerts/{alert_id}/ack when user dismisses/acknowledges

Foreground refresh:
  -> poll GET /v1/households/{id}/devices sparingly (e.g. on pull-to-refresh)
  -> use active_alert_count badge on device rows
```

Push delivery is sent to **all household members** with registered push tokens when alert rules include the `push` channel. SMS is sent for `no_movement_detected` and `device_offline` when rules include `sms` and the member has a verified phone (server-side; no mobile API for SMS prefs yet).

### 8.6 Trial and subscription UI

```text
On device list/detail:
  if subscription.status == "trialing":
    show "Trial ends {current_period_end}"
  if subscription.status == "past_due" or service_status == "service_suspended":
    show "Subscription required" + checkout CTA

Checkout (demo):
  POST .../subscription/checkout
  -> treat status "active" as success
```

Default trial length: **30 days** (`DEFAULT_TRIAL_DAYS`).

---

## 9. Polling and Caching Guidance

| Data | Strategy |
|------|----------|
| Device list status | Pull-to-refresh; avoid background polling < 5 min |
| Alerts | Refresh on screen open; rely on push for urgency |
| Monitored windows | Fetch on settings screen; after PUT, show `delivery_state` |
| User profile | Fetch once per session |
| Token refresh | Proactive before expiry |

No ETags or `If-Modified-Since` support yet.

---

## 10. Not Yet Implemented

These appear in [`mobile-api.md`](mobile-api.md) but have **no route** today:

| Endpoint | Purpose |
|----------|---------|
| `POST /v1/auth/logout-all` | Revoke all sessions |
| `GET /v1/households/{household_id}` | Single household fetch |
| `GET /v1/devices/{id}/timeline` | Event timeline |
| `GET/PUT .../notification-preferences` | Per-device alert toggles, quiet hours |
| `POST .../subscription/portal` | Billing portal |
| OpenAPI 3.1 spec | Machine-readable contract |

Real activity/timeline data will replace the activity stub in a future release.

---

## 11. Environment and Testing

| Environment | Base URL | Notes |
|-------------|----------|-------|
| Local | `http://127.0.0.1:8090` | Requires Docker Postgres; VNMS optional for enrollment |
| Staging / prod | Set per deploy | HTTPS required |

**Health check:** `GET /healthz`

**CORS origins (web only):** `http://127.0.0.1:5173`, `http://localhost:5173` by default.

For full local setup see [`operations.md`](operations.md).

---

## 12. Quick Reference — All Mobile Routes

| Method | Path | Auth | Min role |
|--------|------|:----:|----------|
| GET | `/healthz` | — | — |
| POST | `/v1/auth/signup` | — | — |
| POST | `/v1/auth/login` | — | — |
| POST | `/v1/auth/refresh` | Bearer | — |
| POST | `/v1/auth/logout` | Bearer | — |
| POST | `/v1/auth/verify-email` | — | — |
| POST | `/v1/auth/password-reset/request` | — | — |
| POST | `/v1/auth/password-reset/complete` | — | — |
| GET | `/v1/me` | Bearer | — |
| PATCH | `/v1/me` | Bearer | — |
| GET | `/v1/households` | Bearer | member |
| POST | `/v1/households` | Bearer | — |
| PATCH | `/v1/households/{household_id}` | Bearer | owner/admin |
| GET | `/v1/households/{household_id}/members` | Bearer | member |
| POST | `/v1/households/{household_id}/invites` | Bearer | owner/admin |
| POST | `/v1/invites/{token}/accept` | Bearer | — |
| POST | `/v1/push-tokens` | Bearer | — |
| DELETE | `/v1/push-tokens/{push_token_id}` | Bearer | — |
| GET | `/v1/households/{household_id}/devices` | Bearer | member |
| POST | `/v1/households/{household_id}/devices/register` | Bearer | owner/admin |
| POST | `/v1/households/{household_id}/device-claims` | Bearer | owner/admin |
| GET | `/v1/devices/{device_binding_id}` | Bearer | member |
| PATCH | `/v1/devices/{device_binding_id}` | Bearer | owner/admin |
| POST | `/v1/devices/{device_binding_id}/remove` | Bearer | owner/admin |
| GET | `/v1/devices/{device_binding_id}/monitored-windows` | Bearer | member |
| PUT | `/v1/devices/{device_binding_id}/monitored-windows` | Bearer | owner/admin |
| GET | `/v1/devices/{device_binding_id}/activity` | Bearer | member (stub) |
| GET | `/v1/devices/{device_binding_id}/alerts` | Bearer | member |
| POST | `/v1/devices/{device_binding_id}/alerts/{alert_id}/ack` | Bearer | member |
| GET | `/v1/devices/{device_binding_id}/subscription` | Bearer | member |
| POST | `/v1/devices/{device_binding_id}/subscription/checkout` | Bearer | member (demo) |

---

## 13. Source Code Index

| Concern | Path |
|---------|------|
| Route table | `internal/httpapi/server.go` |
| Auth handlers | `internal/httpapi/auth_handlers.go` |
| Household handlers | `internal/httpapi/household_handlers.go` |
| Device / push / alert handlers | `internal/httpapi/device_handlers.go` |
| Errors, CORS, auth middleware | `internal/httpapi/middleware.go` |
| JSON models | `internal/domain/models.go` |
| Role permissions | `internal/authz/roles.go` |
| Device enrollment & QR parsing | `internal/devices/service.go` |
| Monitored window validation | `internal/schedule/local.go` |
| Push delivery | `internal/notifications/dispatcher.go` |

---

## Related docs

- [`mobile-api.md`](mobile-api.md) — API design direction and future endpoints
- [`auth-security.md`](auth-security.md) — security model
- [`alerts-notifications.md`](alerts-notifications.md) — alert rules and push behavior
- [`device-lifecycle.md`](device-lifecycle.md) — claim, provision, removal
- [`subscriptions-payments.md`](subscriptions-payments.md) — billing design
- [`domain-model.md`](domain-model.md) — entity relationships
