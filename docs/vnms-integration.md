# VNMS Integration

## Purpose

VSRV integrates with VNMS through an authenticated service-to-service API and a
durable event cursor. VNMS owns device-network correctness; VSRV owns product
authorization and mobile behavior.

## Integration Rules

- Mobile clients never call VNMS.
- VSRV calls VNMS only after authorizing the user through household membership.
- VSRV must not persist raw device keys after provisioning.
- VNMS timestamps and monitored-window hours are UTC.
- VSRV stores local-time user intent and converts it to VNMS UTC policy.
- VNMS event delivery is at least once. VSRV must deduplicate.
- VSRV should tolerate VNMS being temporarily unavailable by using pending states
  and retry jobs.

## VNMS API Capabilities

Current VNMS API is documented by `vigelo-nms/docs/vnms-http-api.md` and
`vigelo-nms/internal/httpapi/openapi/openapi.yaml`.

Important endpoints:

```http
GET  /healthz
GET  /v1/devices?q=&limit=50&offset=0
POST /v1/devices:provision
POST /v1/devices:batchGet
GET  /v1/devices/{device_id}
GET  /v1/devices/{device_id}/activity?start=YYYY-MM-DD&end=YYYY-MM-DD
PUT  /v1/devices/{device_id}/monitored-windows
POST /v1/devices/{device_id}/request-device-info
POST /v1/devices/{device_id}/request-device-status
POST /v1/devices/{device_id}/deactivate
GET  /v1/events?after=0&limit=100
```

The legacy VNMS route `PUT /v1/devices/{device_id}/desired-schedule` exists as an
alias but VSRV should use `/monitored-windows`.

## Authentication

Production VNMS deployments should require `Authorization: Bearer <token>`.
VSRV stores this service token in secret management, not in source control.

Use separate credentials per environment:

- local development
- staging
- production

All VSRV calls to VNMS should carry:

- bearer token
- request ID
- service actor identifier if VNMS supports it

Mutating actions should be audited in VSRV before/after the VNMS call and in VNMS
through its own audit/event logs.

## Device Claim and Provisioning Flow

```text
Mobile scans QR
  -> VSRV parses claim data
  -> VSRV authenticates user and household permission
  -> VSRV creates pending DeviceBinding
  -> VSRV calls VNMS POST /v1/devices:provision
  -> VSRV marks binding claimed/provisioned
  -> VSRV starts subscription activation flow if needed
  -> Mobile sees device as pending first contact or active
```

QR/enrollment payload for current-generation devices is expected to contain:

- `device_id`, currently the modem IMEI.
- Enrollment secret or device key material needed to register with VNMS.
- Optional manufacturing metadata.

Rules:

- Validate QR structure and expiry if present.
- Do not log QR payloads.
- Hash or redact secrets in audit records.
- Do not persist raw device keys after successful VNMS provisioning.
- Provisioning should be idempotent for retries.
- If VNMS returns a conflict, surface a product error such as "device already
  claimed or key conflict" and route to support.

## Device State Synchronization

VSRV can read VNMS state synchronously for foreground mobile views:

```http
POST /v1/devices:batchGet
```

Use batch reads for device lists. Avoid one VNMS call per device in mobile list
screens.

VSRV may cache:

- `last_contact_at`
- online/offline derived state
- battery voltage/status
- firmware/modem metadata
- latest modem/payload data counter summaries where product-visible
- monitored-window delivery status
- lifecycle state

Cache source fields with `last_vnms_sync_at`. Treat VNMS as authoritative.

## Activity Reads

VNMS returns UTC daily rows with 24 hourly booleans. VSRV should:

- Request a UTC range wide enough to cover the mobile local-date range.
- Convert UTC hours to the household timezone.
- Return local day/hour structures to mobile.
- Include monitored-hour and monitored-window event facts.
- Keep raw one-minute movement hidden from normal product API.

## Monitored Windows

VSRV owns user intent:

```text
household timezone
local start/end hour(s)
notification rules
user who changed it
desired version
```

VNMS owns device policy:

```json
{
  "monitored_windows": [
    { "start_hour": 5, "duration_hours": 6 }
  ]
}
```

Conversion rules:

- Convert from household local time to UTC hours at write time.
- Keep up to two windows.
- Reject overlapping windows.
- Reject more than two windows.
- Reject 24h plus any other window.
- Represent crossing-midnight local windows as the correct UTC range. If this
  requires more than two UTC windows around DST edges, constrain MVP UI or mark
  policy as needing later DST handling.

Delivery semantics:

- VNMS stores policy immediately.
- Device receives it only on next uplink/downlink.
- VSRV UI should show "pending delivery" until VNMS confirms delivery or enough
  time passes for the next contact.

## Device Info and Device Status

Device-info is static/slow metadata:

- device firmware version
- modem firmware version
- IMEI
- IMSI
- ICCID

Device-status is dynamic daily telemetry:

- battery voltage
- optional modem TX/RX counters from `AT+QGDCNT?`
- firmware-counted payload TX/RX counters

VSRV should display battery in volts, for example `3.000 V`. Data counters are
mainly support/fleet diagnostics, not primary mobile UX unless a user-facing data
usage feature is added.

VSRV can call:

```http
POST /v1/devices/{device_id}/request-device-info
POST /v1/devices/{device_id}/request-device-status
```

These requests are queued for the next device contact.

## Event Cursor

VSRV consumes:

```http
GET /v1/events?after={cursor}&limit=100
```

Store per-consumer state:

- `consumer_name`
- `last_cursor`
- `updated_at`
- `last_error`

Event processing loop:

```text
read events after last cursor
for each event in order:
  begin transaction
    if event already processed: advance cursor
    else apply product projection/alert logic
    record processed event ID/idempotency key
    update cursor
  commit
```

Initial event types from VNMS:

- `movement_uplink.accepted`
- `device_info.received`
- `device_status.received`
- `monitored_window.movement_detected`
- `monitored_window.no_movement_detected`
- `device.policy_delivered`
- `device.lifecycle_changed`

Expected VSRV reactions:

- `monitored_window.movement_detected`: evaluate movement alert rules.
- `monitored_window.no_movement_detected`: evaluate no-movement alert rules.
- `device_info.received`: refresh device metadata cache.
- `device_status.received`: refresh voltage/data telemetry cache and low-battery
  rules once thresholds are defined.
- `device.policy_delivered`: clear pending monitored-window delivery state.
- `device.lifecycle_changed`: update product status and support views.
- `movement_uplink.accepted`: update last activity/last seen projection if needed.

## Failure Handling

Synchronous VNMS call fails:

- Keep VSRV desired state if safe.
- Mark delivery/sync state as `sync_failed`.
- Retry through a background job for idempotent operations.
- Show a user-friendly "will retry" or "try again" state.

Event cursor fails:

- Do not skip cursor.
- Retry with backoff.
- Alert operators if lag exceeds threshold.

VNMS returns duplicate/conflict:

- Treat idempotent success as success.
- For key/device conflicts, stop automatic retries and require support workflow.

## Observability

Track:

- VNMS API latency and error rate.
- Event cursor lag.
- Event processing failures by type.
- Device claim success/failure rate.
- Monitored-window delivery latency.
- Push notification latency after VNMS facts.

## Testing Strategy

- Contract tests against VNMS OpenAPI.
- Fake VNMS client for mobile API tests.
- Event fixture tests for each VNMS event type.
- Idempotency tests for duplicate events.
- Timezone conversion tests for monitored windows and activity ranges.
- Failure/retry tests for VNMS unavailability.
