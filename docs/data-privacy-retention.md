# Data Privacy and Retention

## Purpose

Vigelo handles movement and presence data linked to households. Even though the
device collects only simple PIR movement bits, longitudinal household activity can
be personal data and can become sensitive if used for wellbeing or health-related
inferences.

VSRV owns user/household mapping, product retention, data export, deletion, and
consent boundaries.

## Data Classification

### Account Data

- User email.
- Optional phone.
- Password/session records.
- Household membership.

Protection: personal data, access controlled and auditable.

### Device Metadata

- `device_id`, currently modem IMEI.
- Firmware version.
- Modem firmware version.
- IMSI/ICCID when available.
- Last contact/source metadata.

Protection: protected device metadata once linked to a household.

### Movement and Presence Data

- Hourly movement presence.
- Monitored-window movement/no-movement facts.
- Timeline and alert records.
- Future derived summaries.

Protection: personal data when linked to a household. Avoid raw/minute-level
storage in VSRV unless there is explicit purpose and consent.

### Payment Data

- Payment provider customer/subscription IDs.
- Invoice metadata.
- Subscription status.

Protection: personal data and financial metadata. Do not store card data.

### Push Data

- APNs/FCM tokens.
- Delivery attempts.
- Notification preferences.

Protection: personal data; tokens should be encrypted or otherwise strongly
protected.

## Data Minimization

VSRV should store:

- Product-ready movement summaries and facts.
- Alert state and notification history needed for UX/support.
- Device status cache needed for mobile views.
- Subscription and entitlement state.

VSRV should not store:

- Device AEAD keys after claim.
- Raw device payloads.
- Minute-level movement by default.
- Raw QR/enrollment secrets beyond claim completion.
- Full push tokens in logs.

## Retention Direction

Initial retention proposal:

- Account records: while account is active, then legal/deletion policy.
- Household/device binding: while service is active plus support/legal retention.
- Hourly activity summaries: product-defined retention, initially long enough for
  app history and support.
- Alert/timeline records: bounded, for example 12-24 months unless product needs
  longer.
- Push delivery attempts: short operational retention, for example 30-90 days.
- Auth/session audit: security retention, for example 12 months.
- Payment webhook metadata: provider/legal retention requirements.
- Raw claim secrets: do not retain after successful claim; expire failed claims.

Exact durations should be decided before production, but schemas should include
timestamps needed for lifecycle jobs.

## Export and Deletion

VSRV must be able to export user/household data:

- Account profile.
- Household membership.
- Device bindings.
- Subscription status.
- Alert rules and notification preferences.
- Product movement summaries and timeline records.

VSRV must be able to delete or anonymize:

- User account.
- Household membership.
- Household/device mapping.
- Push tokens.
- Product history according to legal/product policy.

VNMS raw storage may be partitioned by owner/household where possible. VSRV is
the source of truth for the `device_id -> household/owner` mapping needed for
data subject operations.

## Logging and Analytics

Rules:

- Do not send raw `device_id`, IMEI, IMSI, ICCID, QR payloads, or movement details
  to generic analytics tools.
- Prefer device binding IDs or anonymized IDs in product analytics.
- Keep production and staging analytics separate.
- Redact sensitive fields in structured logs.
- Avoid user screenshots or support exports that expose raw identifiers.

## Consent Boundary for AI and Health Inference

Ordinary Vigelo service can use movement facts for monitoring and alerts.

AI/reporting features that infer wellbeing, health, dementia risk, or similar
patterns require separate review. Longitudinal movement patterns can be
quasi-identifying and may become special-category data under GDPR if health
inferences are made.

Before such features:

- Define explicit purpose.
- Collect explicit consent if required.
- Perform DPIA/legal review.
- Separate data pipeline and access controls.
- Avoid medical claims unless regulatory pathway is intentional.

## Security Controls

Required before production:

- TLS for all public APIs.
- Encrypted backups.
- Restore test.
- Secret management.
- Least-privilege database roles.
- Audited admin/support access.
- Retention/deletion jobs.
- Data export procedure.
- Incident response basics.

## Open Decisions

- Exact retention periods.
- Whether user-visible history is hourly, daily, or event-based in MVP.
- Whether VSRV stores any long-term telemetry aggregates beyond VNMS/device cache.
- Owner-prefix strategy for VNMS raw storage and deletion coordination.
- Product wording and consent for future analytics/AI features.
