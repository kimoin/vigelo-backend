# Vigelo Backend Design Docs

**Implementation status:** [`implementation-status.md`](implementation-status.md) — Phases 1–7 deploy ready (2026-07-10).

Read in this order:

0. [`implementation-status.md`](implementation-status.md) — what is built today (API, migrations, E2E).
0. [`operations.md`](operations.md) — **env vars, local dev, UpCloud, troubleshooting**.
0. [`upcloud-deploy.md`](upcloud-deploy.md) — **two-VM UpCloud + VNMS setup**.
0. [`admin-console.md`](admin-console.md) — operator admin UI and API.
1. [`vsrvplan.md`](vsrvplan.md) - consolidated implementation plan, deployment
   topology, and locked-in product decisions.
1. [`architecture.md`](architecture.md) - system boundary and module direction.
2. [`domain-model.md`](domain-model.md) - product entities and authorization shape.
3. [`device-lifecycle.md`](device-lifecycle.md) - claim, provision, transfer, removal.
4. [`vnms-integration.md`](vnms-integration.md) - service-to-service VNMS API and events.
5. [`mobile-api.md`](mobile-api.md) - API contract direction for the mobile app.
6. [`auth-security.md`](auth-security.md) - account, session, authorization, audit.
7. [`subscriptions-payments.md`](subscriptions-payments.md) - service subscription and payment flow.
8. [`alerts-notifications.md`](alerts-notifications.md) - alert policy and push delivery.
9. [`data-privacy-retention.md`](data-privacy-retention.md) - personal data and retention.
10. [`development-plan.md`](development-plan.md) - staged implementation plan.

Source-of-truth dependencies:

- `vigelo-nms/docs/vnms-design.md`
- `vigelo-nms/docs/nms-first-release.md`
- `vigelo-nms/docs/vnms-http-api.md`
- `vigelo-nms/internal/httpapi/openapi/openapi.yaml`
- `vigelo-nms/docs/security-model.md`
- `vigelo-nms/docs/mobile-accounts-subscriptions.md`
- `vigelo-device/specs/payload-format.json`

VSRV should reference VNMS/device protocol docs rather than duplicating low-level
UDP, AEAD, replay-counter, or binary payload details.
