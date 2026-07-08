# Backend Development Plan

## Principles

- Build thin vertical slices.
- Keep VSRV product-focused and mobile-facing.
- Keep VNMS integration behind a typed client.
- Use Go as the default backend language.
- Use PostgreSQL and SQL migrations first.
- Publish OpenAPI early for the mobile API.
- Preserve security and household authorization from the first endpoint.
- Avoid introducing infrastructure before there is operational need.

## Phase 0: Documentation and Contracts

Outputs:

- Architecture/domain/security docs.
- Initial OpenAPI skeleton for mobile API.
- VNMS client contract based on VNMS OpenAPI.
- Local development conventions.

Decisions to make before coding:

- API route naming and resource IDs.
- Auth token/session implementation details.
- Initial database migration tooling.
- Payment provider choice.
- Push provider setup path.

## Phase 1: Project Skeleton

Create:

```text
cmd/vsrv/
internal/config/
internal/httpapi/
internal/auth/
internal/store/
internal/logging/
migrations/
docs/
```

Add:

- `Makefile`
- `.env.example`
- local PostgreSQL compose or documented native setup
- health endpoint
- structured logging
- database migration command
- basic test setup

## Phase 2: Accounts and Sessions

Implement:

- Signup.
- Login.
- Refresh token rotation.
- Logout.
- Email verification placeholder or full flow.
- Password reset placeholder or full flow.
- Session table.
- Rate limits for auth endpoints.

Tests:

- Password hashing.
- Login success/failure.
- Refresh rotation and replay rejection.
- Logout revocation.
- Rate limit behavior.

## Phase 3: Households and Authorization

Implement:

- Household creation.
- Membership table.
- Owner role.
- Authorization middleware/helpers.
- `GET /v1/me`.
- Household list/detail.

Tests:

- User cannot access another household.
- Device routes require membership.
- Role checks are centralized.

## Phase 4: VNMS Client and Device Claim

Implement:

- Typed VNMS client for provision, get, batch get, set monitored windows, activity,
  request info/status, deactivate, events.
- Device claim endpoint.
- Device binding table.
- QR payload parser interface with a development parser.
- Device list and detail endpoints backed by VSRV binding plus VNMS state.

Tests:

- Claim success.
- Claim retry/idempotency.
- VNMS conflict handling.
- Secret redaction.
- Authorization by household/device binding.

## Phase 5: Monitored Windows

Implement:

- Local-time monitored-window intent table.
- Household timezone.
- Validation.
- Conversion to VNMS UTC monitored windows.
- Delivery state.
- Mobile read/write endpoints.

Tests:

- Valid/invalid windows.
- Crossing midnight.
- UTC conversion.
- VNMS failure and retry state.
- Delivery event clears pending state.

## Phase 6: VNMS Event Consumer

Implement:

- Event cursor table.
- Consumer loop.
- Idempotency table.
- Product projections for device info/status and monitored-window facts.
- Internal outbox for alert/notification jobs.

Tests:

- Duplicate event handling.
- Cursor advancement.
- Failure does not skip event.
- Movement/no-movement event produces timeline/alert candidate.

## Phase 7: Alerts and Push

Implement:

- Alert rule table.
- Alert instance table.
- Notification preferences.
- Push token registration.
- APNs/FCM sender abstraction.
- Notification job worker.

Tests:

- Rule evaluation.
- Quiet hours.
- Push token invalidation.
- No duplicate pushes for duplicate VNMS events.

## Phase 8: Subscriptions and Payments

Implement:

- Subscription table.
- Payment customer/subscription references.
- Checkout/session creation.
- Webhook signature verification.
- Entitlement/service state.
- Subscription status endpoint.

Tests:

- Webhook idempotency.
- Subscription activation.
- Past-due/grace behavior.
- Billing authorization.

## Phase 9: Product Hardening

Before production:

- TLS deployment.
- Secret management.
- Backup and restore test.
- Admin/support access model.
- Audit log review.
- Data retention jobs.
- Data export/deletion procedure.
- OpenAPI generated client or contract tests.
- Load test for mobile list/detail and event consumer.

## Implementation Order Recommendation

For the next concrete coding work:

1. Project skeleton and health endpoint.
2. Database migrations and auth/session basics.
3. Household/device binding model.
4. VNMS client and device claim.
5. Device list/detail mobile API.
6. Monitored-window edit flow.
7. VNMS event consumer.
8. Alerts/push.
9. Subscriptions/payments.

This order gives a usable mobile prototype while keeping the core ownership and
VNMS boundaries correct.
