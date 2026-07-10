# Mobile API Design

> **For app developers:** see [`mobile-developer-guide.md`](mobile-developer-guide.md) for the
> full reference of **implemented** endpoints, request/response shapes, enums, roles, and
> integration flows. This document describes design direction and planned endpoints.

## Purpose

The VSRV mobile API is the only API used by the Vigelo mobile app. It exposes
product concepts, not VNMS internals.

Mobile API clients should not know about:

- VNMS hostnames.
- UDP or NB-IoT behavior.
- AEAD, `key_id`, boot counters, or message counters.
- Device binary payloads.
- Raw IMEI/device IDs except in controlled claim/support contexts.

## API Style

Recommended first implementation:

- HTTPS JSON API.
- OpenAPI 3.1 specification stored in this repository.
- Versioned under `/v1`.
- UTC RFC3339 timestamps.
- Stable opaque IDs for VSRV resources.
- Structured error envelope.
- Idempotency keys for payment and claim mutations where retries are expected.

Error shape:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "Human readable message",
    "field": "email"
  }
}
```

Common codes:

- `invalid_request`
- `unauthorized`
- `forbidden`
- `not_found`
- `conflict`
- `rate_limited`
- `subscription_required`
- `device_not_ready`
- `internal_error`

## Authentication Endpoints

Initial account flow:

```http
POST /v1/auth/signup
POST /v1/auth/login
POST /v1/auth/refresh
POST /v1/auth/logout
POST /v1/auth/logout-all
POST /v1/auth/verify-email
POST /v1/auth/password-reset/request
POST /v1/auth/password-reset/complete
GET  /v1/me
PATCH /v1/me
```

Initial token direction:

- Short-lived opaque access token or session token.
- Long-lived refresh token stored only as a hash server-side.
- Refresh token rotation on every refresh.
- Revocation on logout and security events.
- Mobile stores tokens in iOS Keychain or Android secure storage.

## Household Endpoints

```http
GET  /v1/households
POST /v1/households
GET  /v1/households/{household_id}
PATCH /v1/households/{household_id}
GET  /v1/households/{household_id}/members
POST /v1/households/{household_id}/invites
```

MVP may auto-create one household during onboarding. Keep the API model ready for
multiple households and invited caregivers.

## Device Claim and Device List

```http
POST /v1/households/{household_id}/device-claims
GET  /v1/households/{household_id}/devices
GET  /v1/devices/{device_binding_id}
PATCH /v1/devices/{device_binding_id}
POST /v1/devices/{device_binding_id}/remove
```

Claim request example:

```json
{
  "qr_payload": "vigelo://claim?...",
  "display_name": "Living room",
  "room_or_location_label": "Living room"
}
```

Claim behavior (implemented):

1. Parse and validate QR payload or structured `device_id` + `enrollment_secret`.
2. Verify the user can claim into the household.
3. Call VNMS `POST /v1/devices/{device_id}/verify-enrollment` (constant-time key check).
4. Create Postgres `device_bindings` and trialing `subscriptions`.
5. Call VNMS `POST /v1/devices/{device_id}/enable`.
6. Merge VNMS state via `batchGet` for list/detail responses.
7. Prompt for subscription activation if needed.

Factory inventory must exist in VNMS (`POST /v1/devices:provision-inventory`) before
user enrollment. VSRV does **not** call `POST /v1/devices:provision` during claim.

Never return the device key to the app after claim.

Device list response should be presentation-friendly:

```json
{
  "devices": [
    {
      "id": "devbind_123",
      "display_name": "Living room",
      "status": "online",
      "last_seen_at": "2026-07-08T12:00:00Z",
      "battery_voltage_v": 3.0,
      "subscription_status": "active",
      "monitored_windows_summary": "08:00-20:00",
      "pending_policy_delivery": false
    }
  ]
}
```

## Device Detail

```http
GET /v1/devices/{device_binding_id}
```

The detail response should combine:

- VSRV binding: name, household, subscription, user policy.
- VNMS state: last seen, latest voltage, metadata, delivery status.
- Derived product status: online/offline, battery state, service active/inactive.
- Alert state: active alerts, latest monitored-window outcome.

Recommended fields:

- `id`
- `display_name`
- `status`
- `last_seen_at`
- `battery_voltage_v`
- `battery_status`
- `firmware_version`
- `modem_firmware_version`
- `monitored_windows`
- `monitored_windows_delivery_state`
- `active_alerts`
- `subscription`

Keep raw `device_id` out of normal detail responses unless a support/debug flag is
used.

## Monitored Windows

```http
GET /v1/devices/{device_binding_id}/monitored-windows
PUT /v1/devices/{device_binding_id}/monitored-windows
```

Mobile-facing payload should use local-time intent:

```json
{
  "timezone": "Europe/Helsinki",
  "windows": [
    {
      "start_time": "08:00",
      "end_time": "20:00"
    }
  ],
  "alert_mode": "no_movement_detected"
}
```

`alert_mode` is **mutually exclusive** — choose one:

| Value | Meaning |
|-------|---------|
| `no_movement_detected` | Alert if no movement during monitored window (default) |
| `movement_detected` | Alert if movement is detected during monitored window |

Saving monitored windows enables the selected movement rule and disables the other.
`device_offline` alerts are independent of this choice.

First VNMS release supports daily repeating UTC windows, not per-weekday windows.
For MVP, VSRV can constrain the UI/API to daily repeating windows. Keep `days`
out of the first implementation if it cannot be delivered accurately yet.

Write behavior:

1. Validate product intent and household permissions.
2. Convert local-time window(s) into UTC `start_hour`/`duration_hours` for VNMS.
3. Call VNMS `PUT /v1/devices/{device_id}/monitored-windows`.
4. Store desired intent and mark delivery pending.
5. Update delivery status when VNMS emits `device.policy_delivered` or state read
   confirms delivery.

## Activity and Timeline

```http
GET /v1/devices/{device_binding_id}/activity?start=2026-07-01&end=2026-07-09
GET /v1/devices/{device_binding_id}/timeline?limit=50
```

Activity should be returned in the household timezone by default, even if VNMS
stores UTC days/hours.

Recommended activity shape:

```json
{
  "timezone": "Europe/Helsinki",
  "days": [
    {
      "date": "2026-07-08",
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

First release should present hourly movement presence and monitored-window facts.
Do not expose minute-level data unless a separate consented research/data export
regime exists.

## Alerts and Notification Preferences

```http
GET /v1/devices/{device_binding_id}/alerts
POST /v1/devices/{device_binding_id}/alerts/{alert_id}/ack
GET /v1/devices/{device_binding_id}/notification-preferences
PUT /v1/devices/{device_binding_id}/notification-preferences
```

Notification preferences should include:

- Movement/no-movement alert toggles.
- Device offline alert toggle.
- Low battery alert toggle.
- Quiet hours.
- Recipient selection when household roles exist.
- Push channel enabled state.

VNMS emits facts. VSRV decides whether each fact becomes a push notification.

## Subscription Endpoints

```http
GET  /v1/devices/{device_binding_id}/subscription
POST /v1/devices/{device_binding_id}/subscription/checkout
POST /v1/devices/{device_binding_id}/subscription/portal
```

The mobile app should see service status and next action, not payment-provider
internals.

## Push Token Endpoints

```http
POST   /v1/push-tokens
DELETE /v1/push-tokens/{push_token_id}
```

The app registers APNs/FCM tokens with VSRV. VSRV stores tokens securely and uses
them only according to household membership and notification preferences.

## Admin and Support API

Keep admin/support APIs separate from mobile APIs:

- Stronger authentication.
- MFA before production.
- Audit all reads and writes.
- Support access scoped and time-bound.
- Avoid broad raw movement access.

## Pagination and Caching

- Use cursor pagination for timelines and events.
- Limit offset pagination to operator/support tables.
- Use ETags or `updated_at` fields later if mobile performance needs it.
- Mobile should poll sparingly; push and foreground refresh should carry most UI
  freshness.

## OpenAPI Requirement

Create and maintain an OpenAPI spec before implementation grows. It should be the
contract for:

- Mobile app development.
- Mock server generation.
- API tests.
- Backward-compatible mobile releases.
