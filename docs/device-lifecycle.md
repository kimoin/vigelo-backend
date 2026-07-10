# Device Lifecycle

> **Implementation status:** Enrollment is live: VSRV provisions the device in
> VNMS on first claim (admin or mobile), then enables it. See
> [`implementation-status.md`](implementation-status.md).

## Purpose

VSRV owns the product lifecycle of a physical Vigelo device in a household. VNMS
owns the device-network lifecycle. The two are related but not identical.

## Product Lifecycle

Suggested VSRV lifecycle:

```text
unclaimed
  -> claim_started
  -> claimed
  -> provisioned_in_vnms
  -> subscription_pending
  -> active
  -> suspended
  -> removed
  -> transferred
```

VNMS lifecycle is lower-level:

```text
development / active / revoked
```

VSRV should map VNMS state into user-facing status. Do not expose VNMS lifecycle
raw in normal mobile screens.

## Claim Flow

```text
User enters device_id + 32-character hex key (mobile app or VSRV admin)
  -> VSRV validates input
  -> VSRV calls VNMS verify-enrollment
     - unknown or unprovisioned in NMS -> VSRV calls provision-inventory (disabled)
     - wrong key -> rejected
     - already active in NMS -> conflict
  -> VSRV creates DeviceBinding + trialing subscription
  -> VSRV calls VNMS enable
  -> user activates subscription (demo checkout)
  -> device becomes active when service and first contact are ready
```

Claim inputs:

- `device_id` — any alphanumeric identifier (1–64 characters), commonly modem IMEI.
- `enrollment_secret` — 32-character hex device key (16 bytes).

Claim requirements:

- Authenticated user.
- Household permission to claim.
- Idempotent retry behavior.
- Secret redaction in logs/audit.
- No long-term VSRV storage of raw device key after successful VNMS provisioning.

## Provisioning in VNMS

NMS learns about a device for the first time when VSRV enrolls it (mobile claim or
admin provision). There is no separate factory pre-import step in the product flow.

**VSRV enrollment (internal sequence):**

1. `POST /v1/devices/{device_id}/verify-enrollment` — key check when device already has credentials in NMS.
2. If the device is unknown or **unprovisioned** in NMS, VSRV calls:

```http
POST /v1/devices:provision-inventory
```

```json
{
  "device_id": "860123456789012",
  "device_key": "000102030405060708090a0b0c0d0e0f"
}
```

Device is stored as `disabled` until enable.

3. After the VSRV binding is created:

```http
POST /v1/devices/{device_id}/enable
```

VNMS stores the device key and uses it to authenticate future UDP traffic. VSRV
stores only the device binding and product ownership; raw keys are not persisted
after successful enrollment.

**Removing test or support devices from NMS:**

```http
POST /v1/devices/{device_id}/unprovision
DELETE /v1/devices/{device_id}
```

Permanent delete requires the device to be unprovisioned first.

## First Contact

After provisioning, the device may not have contacted VNMS yet. Mobile UX should
show:

- "Device claimed."
- "Waiting for first device contact."
- Troubleshooting guidance if delayed.

VSRV can read VNMS state by `GET /v1/devices/{device_id}` or batch reads.

## Subscription Activation

A claimed device may need an active service subscription before alerts and full
history are enabled.

Keep separate:

- Device claimed/provisioned state.
- Subscription/payment state.
- Device online/offline state.

## Monitored Windows After Claim

After claim and service activation, VSRV should let the user configure monitored
hours. Until then, VNMS/device may have no monitored windows and only the device
max-silence baseline applies.

Policy delivery is delayed until the next device contact. VSRV stores desired
local-time intent and marks delivery pending until VNMS confirms.

## Removal

Removal can mean different things:

- Hide/remove from household but keep VNMS active temporarily.
- Deactivate/revoke in VNMS so device can no longer authenticate.
- Transfer to another household.
- Replace device under the same subscription.

MVP removal should be conservative:

1. Confirm with user.
2. Stop alert delivery.
3. Mark binding removed.
4. Call VNMS deactivate/revoke if the device should no longer be accepted.
5. Preserve history according to retention policy.

## Transfer and Replacement

Design for later:

- Transfer device ownership to another household.
- Replace physical device but keep subscription/history.
- Re-claim a returned/refurbished device after support workflow.

Do not make product subscription IDs or history primary keys depend directly on
`device_id`.

## Support Cases

Support workflows may need:

- View redacted raw `device_id`.
- Retry VNMS provision.
- Request device info/status refresh.
- Deactivate/revoke.
- Reset replay baseline in VNMS for factory/bench recovery only.

Every support action must be audited.
