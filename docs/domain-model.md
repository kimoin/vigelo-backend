# VSRV Domain Model

## Purpose

This document defines the product-domain model for VSRV. It intentionally sits
above the VNMS device-network model. VNMS knows devices and device facts. VSRV
knows users, households, subscriptions, and what a logged-in mobile user is
allowed to see or change.

## Core Entities

### User

A person with a Vigelo account.

Suggested fields:

- `id`
- `email`
- `email_verified_at`
- `phone` and `phone_verified_at` later
- `display_name`
- `created_at`
- `disabled_at`
- security timestamps such as `last_login_at`

Users do not own devices directly. Users gain access through household/site
membership.

### Household / Site

The product ownership container. It may represent a home, care site, apartment,
or monitored location.

Suggested fields:

- `id`
- `name`
- `timezone`
- `country`
- `created_by_user_id`
- `created_at`
- `archived_at`

Store a timezone on the household because monitored hours and notification quiet
hours are user/product concepts. VNMS stores UTC policy only.

### Membership

Connects a user to a household with a role.

Initial roles:

- `owner`: can manage household, devices, members, subscriptions, and billing.
- `admin`: can manage devices and alert rules, but not necessarily billing.
- `member`: can view status and receive notifications.
- `caregiver`: can view and receive alerts, with limited configuration rights.
- `support`: not a normal membership; support access should be time-bound and
  audited separately.

MVP can start with only `owner`, but the schema and authorization layer should
not assume a single user forever.

### Device Binding

The VSRV-owned relationship between a household and a VNMS `device_id`.

Suggested fields:

- `id`
- `household_id`
- `device_id`
- `display_name`
- `room_or_location_label`
- `claim_status`
- `claimed_by_user_id`
- `claimed_at`
- `removed_at`
- `removed_reason`
- `vnms_lifecycle_state_cache`
- `last_vnms_sync_at`

For the current product generation, `device_id` is the modem IMEI. Treat it as
protected device metadata after claim. Do not use it as the only mobile-facing ID.
Mobile APIs should normally use the VSRV device binding ID.

### Device Claim

Represents a claim attempt from QR/enrollment data.

Suggested fields:

- `id`
- `household_id`
- `device_id`
- `claim_token_hash` or parsed enrollment reference
- `requested_by_user_id`
- `status`
- `failure_reason`
- `created_at`
- `completed_at`
- `expires_at`

The QR code may contain the current-generation `device_id` and enrollment/key
material needed to register the device with VNMS. VSRV may handle this secret
during claim, but must not persist raw device keys longer than needed.

### Subscription

The service entitlement for a physical Vigelo monitoring device.

Suggested fields:

- `id`
- `household_id`
- `device_binding_id`
- `status`
- `plan_code`
- `payment_provider`
- `provider_customer_id`
- `provider_subscription_id`
- `current_period_start`
- `current_period_end`
- `trial_ends_at`
- `cancel_at`
- `cancelled_at`

Subscription status drives product access and, where needed, VNMS service policy.
VNMS should not know why a service is active or inactive.

### Monitored Window Intent

User-facing monitored hours for a device.

Suggested fields:

- `id`
- `device_binding_id`
- `timezone`
- `windows_json`
- `enabled`
- `desired_version`
- `last_sent_to_vnms_at`
- `last_delivered_by_vnms_at`
- `delivery_state`
- `created_by_user_id`
- `updated_by_user_id`

VSRV stores the local-time intent. Before writing VNMS policy, VSRV converts it
to UTC monitored windows compatible with VNMS constraints:

- Up to two windows.
- Non-overlapping.
- `start_hour` 0..23.
- `duration_hours` 1..24.
- A 24-hour window must be the only window.

When local-time windows cross daylight-saving transitions, VSRV should preserve
user intent and recompute UTC policy when needed. For MVP, hourly windows and
household timezone are enough; document any DST limitations in the UI.

### Alert Rule

User-facing rule that decides whether a device fact becomes an alert.

Initial rule categories:

- Movement detected during monitored window.
- No movement during monitored window.
- Device offline or missed expected contact.
- Low battery.
- Poor coverage or repeated send failures.
- Schedule delivery problem.

Suggested fields:

- `id`
- `device_binding_id`
- `type`
- `enabled`
- `severity`
- `quiet_hours`
- `channels`
- `recipients`
- `created_by_user_id`
- `updated_by_user_id`

### Alert Instance

A concrete alert state visible to users.

Suggested fields:

- `id`
- `device_binding_id`
- `rule_id`
- `type`
- `severity`
- `status`
- `source_event_id`
- `first_seen_at`
- `last_seen_at`
- `resolved_at`
- `acknowledged_at`
- `acknowledged_by_user_id`

Alert instances are VSRV product records. VNMS emits facts; VSRV turns them into
alerts according to household policy and subscription state.

### Mobile Session

Represents a logged-in app session.

Suggested fields:

- `id`
- `user_id`
- `refresh_token_hash`
- `rotated_from_session_id`
- `device_label`
- `platform`
- `app_version`
- `last_seen_at`
- `revoked_at`

Prefer opaque server-side tokens and refresh-token rotation initially. Keep the
model compatible with future OIDC if needed.

### Push Token

APNs/FCM token registered by the mobile app.

Suggested fields:

- `id`
- `user_id`
- `session_id`
- `platform`
- `token_hash`
- `token_encrypted`
- `environment`
- `enabled`
- `last_registered_at`
- `last_delivery_error`

The app registers push tokens with VSRV, never VNMS.

### Audit Log

Security-sensitive and support-sensitive actions.

Audit at least:

- Login/logout/session revocation.
- Email verification and password reset.
- Household membership changes.
- Device claim, transfer, removal, deactivate/revoke.
- Monitored-window changes.
- Alert rule changes.
- Subscription activation/cancel.
- Support/admin reads and writes.
- VNMS replay-baseline reset if exposed through VSRV support tooling.

## Authorization Model

Every device request must resolve through:

```text
authenticated user
  -> active household membership
  -> role permission
  -> device binding in that household
  -> subscription/service state where relevant
```

Never authorize a user-facing request using only `device_id`.

Recommended permission groups:

- `household.read`
- `household.manage`
- `members.manage`
- `devices.claim`
- `devices.read`
- `devices.configure`
- `devices.remove`
- `alerts.read`
- `alerts.manage`
- `billing.manage`
- `support.read`
- `support.write`

## Device Lifecycle in VSRV

Suggested product lifecycle:

```text
claim_started
  -> claimed
  -> subscription_pending
  -> active
  -> suspended
  -> removed
  -> transferred
```

VNMS has its own network lifecycle such as active/revoked/development. VSRV
should map VNMS lifecycle into product status, not expose it raw unless in
support views.

## MVP Scope

Acceptable MVP simplifications:

- One household per user.
- One device per household.
- Owner role only.
- One active subscription plan.
- Simple alert rules.
- Email/password auth only.

Do not bake these into authorization or database constraints in a way that blocks
the planned model.
