# VSRV Architecture

## Purpose

Vigelo Backend (VSRV) is the product backend between the mobile app and the
device-network layer. It owns user-facing state and policy. Vigelo NMS (VNMS)
owns device-network correctness.

The mobile app calls VSRV only. It must never call VNMS directly.

## Service Boundary

```text
Mobile app
  -> VSRV mobile API
     -> users, households, subscriptions, alert preferences, push delivery
     -> product activity/status views
     -> VNMS service-to-service API for device actions and facts
        -> device authentication, counters, uplinks, downlinks, monitored-window delivery
```

VSRV owns:

- User accounts and mobile sessions.
- Email verification, password reset, session revocation, and later OIDC/MFA if
  needed.
- Households/sites and membership roles.
- Device claiming, ownership, transfer, and removal workflows.
- Subscription and payment state.
- User-facing monitored-hour intent in local time.
- Alert rules, quiet hours, notification preferences, and push token management.
- User-facing movement summaries, status cards, timelines, and support views.
- Authorization checks for every user-facing device read or write.
- Mapping between `device_id` and household/user ownership.
- Product data retention, user export, and deletion workflows.

VNMS owns:

- Device UDP ingress and immediate authenticated downlink.
- Per-device key material.
- AEAD verification, replay protection, boot/msg counters, and duplicate handling.
- Device operational state, last contact, firmware/SIM metadata, voltage, data
  counters, and monitored-window delivery status.
- UTC monitored-window policy as delivered to the device.
- Hourly movement masks and monitored-window movement/no-movement facts.
- Durable outbox events for VSRV.

## Key Constraints From VNMS and Device Design

- Current-generation `device_id` is the modem IMEI. It is globally unique and not
  secret, but once linked to a household it is protected device metadata.
- Keep the field name `device_id`, not `imei`, so later hardware can use another
  product identity without changing API shape.
- Devices do not send wall-clock timestamps. VNMS aligns movement to UTC wall time.
- Movement is one bit per minute on the device, but first-release product storage
  is coarser: hourly movement presence plus monitored-window facts.
- Battery voltage and data counters arrive through a low-rate status uplink. VNMS
  stores one daily latest sample for 31 days and older monthly aggregates.
- Monitored windows are limited to up to two non-overlapping ranges. VSRV stores
  user intent in local time; VNMS stores and pushes validated UTC policy.
- Downlink delivery happens only after a device uplink. Any user change should be
  shown as "pending delivery" until VNMS/device state confirms delivery.
- Push notifications belong in VSRV. VNMS emits facts; VSRV decides whether and
  how to notify users.

## Logical Modules

Start as one Go service with internal modules. Split only when real operational
pressure appears.

```text
cmd/vsrv/
internal/config/
internal/httpapi/
internal/auth/
internal/accounts/
internal/households/
internal/devices/
internal/subscriptions/
internal/payments/
internal/alerts/
internal/notifications/
internal/vnmsclient/
internal/events/
internal/store/
internal/audit/
internal/logging/
migrations/
docs/
```

Suggested module responsibilities:

- `auth`: signup/login/session/token lifecycle, email verification, password reset.
- `accounts`: user profile and account state.
- `households`: household/site records, memberships, roles, invitations.
- `devices`: claim, ownership binding, product device state, local-time policies.
- `vnmsclient`: typed VNMS API client and event cursor consumer.
- `subscriptions`: entitlement and service activation state.
- `payments`: payment provider integration and webhooks.
- `alerts`: alert rule evaluation and user-facing alert state.
- `notifications`: APNs/FCM push registration and delivery.
- `events`: durable ingestion from VNMS and internal outbox for push/email jobs.
- `store`: PostgreSQL persistence and migrations.
- `audit`: security-sensitive and support-sensitive audit records.

## Data Ownership

VSRV is authoritative for:

- User identity.
- Household membership.
- Which household owns which `device_id`.
- Whether a subscription is active.
- Local-time monitored-hour intent and notification preferences.
- User-visible alert state.
- Push token state.
- Payment/customer references.

VNMS is authoritative for:

- Whether a device is provisioned for network access.
- Latest device contact and telemetry.
- Device lifecycle in the network layer.
- Device policy delivery and replay/counter state.
- Raw/operational movement facts from the device.

VSRV should copy or cache VNMS-derived state only when it needs product queries,
mobile performance, notifications, or historical display. Such copies must be
treated as derived and refreshed from VNMS events/API.

## Product Data Model Shape

Minimum product entities:

```text
User
  -> Household / Site
       -> Membership / Role
       -> DeviceBinding
            -> DeviceSubscription
            -> MonitoredWindowIntent
            -> AlertRule / NotificationPreference
            -> DeviceViewCache
MobileSession
PushToken
PaymentCustomer
PaymentSubscription
AuditLog
VNMSCursor
```

The MVP can support one user, one household, one device, and one active
subscription, but schema and authorization should allow:

- Multiple devices per household.
- Multiple users per household.
- Owner/admin/member/caregiver roles.
- Device transfer and replacement.
- Subscription transfer or cancellation.
- Support/admin access with audit.

## Mobile API Principles

- Mobile APIs are user/household scoped, never raw device scoped.
- Use stable product IDs in URLs where possible. Avoid exposing raw `device_id`
  unless support/debugging requires it.
- Store all timestamps in UTC. Accept and return user-facing scheduling intent
  with timezone context.
- Keep response objects presentation-friendly: battery in volts, last seen as UTC
  timestamp plus status hints, monitored windows in local-time intent, delivery
  status separate from desired state.
- Do not expose per-device cryptographic keys after claim/provisioning.
- Use structured errors with machine-readable codes.
- Publish OpenAPI for the mobile API before implementation grows.

## Integration Pattern With VNMS

VSRV uses VNMS in two ways:

1. Synchronous service-to-service API calls for commands and reads:
   provisioning, deactivate/revoke, get/batch device state, set monitored windows,
   read activity, request device info/status.
2. Durable event cursor for facts:
   device info/status received, monitored-window movement/no-movement,
   lifecycle changes, policy delivered, and future low-battery/missed-contact
   events.

VSRV must be idempotent. VNMS event delivery is at least once, so VSRV stores the
last cursor and deduplicates by event ID or idempotency key.

## Deployment Direction

Initial deployment can be simple:

- One Go service.
- PostgreSQL.
- SQL migrations.
- TLS reverse proxy.
- Payment provider webhooks.
- APNs/FCM credentials.
- Service token for VNMS.
- Structured logs and audit logs.

Do not introduce Kubernetes, a streaming platform, or microservices before there
is load or operational need. Preserve module boundaries so these can be split
later.

## Non-Goals

VSRV should not:

- Decode device binary payloads.
- Store device AEAD keys long term.
- Own replay counters or downlink construction.
- Call UDP devices directly.
- Put payment/subscription rules into VNMS.
- Let mobile clients address devices only by raw `device_id`.
- Decide low-level battery or radio behavior. It can express product policy; VNMS
  translates it into safe device policy.
