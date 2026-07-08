# Subscriptions and Payments

## Purpose

VSRV owns subscription state, payment integration, customer records, and service
activation. VNMS must remain payment-blind. VNMS receives only the device service
policy or activation state that affects device handling.

The subscription is for a physical Vigelo monitoring device and data service, not
a generic digital-only app feature.

## Product Direction

Preferred positioning:

- "Activate monitoring service for your Vigelo device."
- "Device data service subscription."
- "Cellular-connected Vigelo monitoring service."

Avoid framing the payment as unlocking premium app-only features, because mobile
platform payment rules can differ for physical goods/services versus digital-only
features.

Before public launch, review Apple App Store and Google Play policies against the
exact product wording and payment flow.

## Ownership

VSRV owns:

- Payment customer references.
- Payment subscriptions.
- Billing status.
- Entitlements.
- Invoices/customer portal links where exposed.
- Payment provider webhooks.
- Service activation/deactivation.

VNMS owns:

- Network device lifecycle.
- Device key material.
- Device delivery and operational state.

VNMS should not know:

- Payment provider.
- Card/payment method details.
- Invoice state.
- Trial or discount details.
- Why a device service is active/inactive.

## Suggested Subscription States

Product subscription states:

- `none`
- `trialing`
- `active`
- `past_due`
- `paused`
- `cancel_scheduled`
- `cancelled`
- `expired`

Derived service states:

- `service_active`
- `service_limited`
- `service_suspended`
- `service_removed`

Keep billing state and service state separate. This allows grace periods and
safety-critical visibility even when billing is past due.

## Payment Provider

Stripe is the likely first candidate because it supports:

- Subscriptions.
- Hosted checkout.
- Customer portal.
- Payment method management.
- Invoices.
- Webhooks.
- Mobile-friendly payment flows.

Do not tightly couple the domain model to Stripe names. Store provider references
behind generic fields:

- `payment_provider`
- `provider_customer_id`
- `provider_subscription_id`
- `provider_checkout_session_id`
- `provider_price_id`

## Activation Flow

```text
Mobile app
  -> requests subscription checkout for device binding
VSRV
  -> verifies household role and device binding
  -> creates provider checkout/payment session
Mobile app
  -> completes payment flow
Payment provider
  -> sends signed webhook
VSRV
  -> verifies webhook signature
  -> updates subscription and entitlement
  -> marks device service active
  -> updates mobile-visible state
  -> optionally informs VNMS of service policy if VNMS needs it
```

VSRV should treat webhooks as authoritative for payment completion, not mobile
client redirects.

## Mobile API

Suggested endpoints:

```http
GET  /v1/devices/{device_binding_id}/subscription
POST /v1/devices/{device_binding_id}/subscription/checkout
POST /v1/devices/{device_binding_id}/subscription/portal
POST /v1/webhooks/payments/{provider}
```

Checkout response:

```json
{
  "checkout_url": "https://...",
  "expires_at": "2026-07-08T12:00:00Z"
}
```

Subscription response:

```json
{
  "status": "active",
  "service_status": "service_active",
  "plan_code": "device_monitoring_monthly",
  "current_period_end": "2026-08-08T00:00:00Z",
  "next_action": null
}
```

## Entitlements

Initial entitlement checks:

- Can the user view current device status?
- Can the user edit monitored windows?
- Should alert notifications be sent?
- Should advanced history/analytics be visible later?

Recommendation:

- Keep basic safety/status visibility available for a grace period after billing
  trouble.
- Gate new alert delivery and history depth by service state if needed.
- Do not let VNMS payment state block device safety behavior unless intentionally
  designed.

## Device Transfer and Subscription Transfer

Support later:

- Transfer device to another household.
- Cancel subscription but preserve history for retention period.
- Replace device under same subscription.
- Move subscription from old device binding to new device binding.

Do not make subscription primary key equal to `device_id`.

## Webhook Processing

Webhook rules:

- Verify provider signature.
- Persist raw provider event metadata with bounded retention.
- Process idempotently by provider event ID.
- Update subscription in a database transaction.
- Emit internal outbox event for service activation/deactivation.
- Never trust mobile callback alone.

## VNMS Policy Impact

Initially, VNMS may not need subscription state. If product policy requires
network-level behavior, VSRV can send a service policy such as:

```text
service_active true/false
allowed_reporting_policy
monitored_window_policy
```

Keep this minimal. VNMS should not implement billing logic.

## Open Decisions

- Exact payment provider.
- Trial length and plan codes.
- Grace period behavior.
- Whether app-store in-app purchases are required for any future digital-only
  feature.
- How device replacement affects subscription terms.
