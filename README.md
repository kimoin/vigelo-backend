# Vigelo Backend (VSRV)

Vigelo Backend, or VSRV, is the product backend for Vigelo. It serves the mobile
application, owns user accounts and household/device ownership, manages
subscriptions and payments, evaluates alert policy, sends push notifications, and
integrates with Vigelo NMS (VNMS) for device-network actions.

VNMS owns the device plane: UDP, AEAD, replay counters, downlinks, monitored-window
delivery, device telemetry ingestion, and durable device facts. VSRV owns the
product plane: users, households, subscriptions, authorization, local-time user
intent, alert rules, mobile sessions, and user-facing APIs.

## Design Documents

- [`docs/architecture.md`](docs/architecture.md) - service boundaries, modules,
  data ownership, and deployment direction.
- [`docs/domain-model.md`](docs/domain-model.md) - users, households, devices,
  subscriptions, alert rules, sessions, and support access.
- [`docs/mobile-api.md`](docs/mobile-api.md) - VSRV-facing mobile API shape and
  response semantics.
- [`docs/vnms-integration.md`](docs/vnms-integration.md) - how VSRV provisions,
  controls, reads, and consumes events from VNMS.
- [`docs/auth-security.md`](docs/auth-security.md) - account security,
  authorization, service authentication, and audit rules.
- [`docs/subscriptions-payments.md`](docs/subscriptions-payments.md) - device
  service subscription and payment flow.
- [`docs/alerts-notifications.md`](docs/alerts-notifications.md) - alert policy,
  notification preferences, and APNs/FCM delivery.
- [`docs/data-privacy-retention.md`](docs/data-privacy-retention.md) - movement
  data protection, retention, export, and deletion direction.
- [`docs/development-plan.md`](docs/development-plan.md) - staged implementation
  plan for the backend.

## Implementation Direction

Go is the default backend language for Vigelo services. Start as one deployable
with clear internal module boundaries, PostgreSQL, SQL migrations, OpenAPI for the
mobile API, and a small VNMS client generated or checked against the VNMS OpenAPI
spec.

Do not push device protocol concepts into the mobile API. The mobile app should
never know about UDP, AEAD, boot counters, modem command details, or binary
payload formats.
