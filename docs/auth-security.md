# Authentication and Security Design

## Purpose

VSRV owns user identity, mobile sessions, household authorization, subscription
entitlements, and support/admin access. Security must be present from the first
implementation because movement and presence data linked to a household can be
personal data.

## Security Boundary

```text
VDEV -> VNMS -> VSRV -> Mobile app
```

- VNMS authenticates devices and protects the device protocol.
- VSRV authenticates users and protects product data.
- Mobile app stores tokens securely and never handles device protocol secrets.

## User Authentication

Initial recommended approach:

- VSRV-owned email/password authentication.
- Password hashes with Argon2id.
- Email verification before production use.
- Password reset with short-lived single-use tokens.
- Server-side sessions with opaque tokens.
- Refresh token rotation.
- Session revocation.
- Rate limits for signup, login, refresh, password reset, and verification.

Commercial OAuth providers are not required for MVP. Keep the model compatible
with later OIDC if integrations or enterprise requirements appear.

## Mobile Token Model

Use two-token mobile sessions:

- Short-lived access token or opaque session token.
- Long-lived refresh token, rotated on every refresh.

Server stores:

- Session ID.
- Refresh token hash, never plaintext.
- User ID.
- Mobile platform/app version/device label.
- Last seen timestamp.
- Revoked timestamp.

Mobile stores tokens in:

- iOS Keychain.
- Android Keystore-backed secure storage.

Logout revokes the current refresh token. "Logout all" revokes all sessions for
the user.

## Authorization

All product authorization should resolve through household membership:

```text
User
  -> Household membership
    -> Role permissions
      -> Device binding
        -> Subscription/service state
```

Rules:

- Never authorize a mobile request by raw `device_id` alone.
- Every device read/write must verify the device binding belongs to a household
  the user can access.
- Support/admin access must be explicit, role-gated, and audited.
- Subscription state can limit service actions, but do not hide safety-critical
  device state solely because billing failed.

## Roles

MVP can implement only `owner`. Keep permission names ready for:

- `owner`
- `admin`
- `member`
- `caregiver`
- `support`

Suggested permission checks:

- `devices.read`
- `devices.configure`
- `devices.claim`
- `devices.remove`
- `alerts.read`
- `alerts.manage`
- `billing.manage`
- `members.manage`
- `support.read`
- `support.write`

## Service-to-Service Security

VSRV calls VNMS with a service token:

```http
Authorization: Bearer <token>
```

Rules:

- Store VNMS token in secret management.
- Use different tokens per environment.
- Rotate tokens before production.
- Send request IDs.
- Do not expose VNMS token to mobile clients.
- Treat VNMS API as internal/private network where practical.

Payment provider webhooks must be verified with provider signatures.

APNs/FCM credentials must be stored as secrets and scoped per environment.

## Device Claim Secret Handling

QR/enrollment data may include device key material or an enrollment secret. VSRV
may handle this during claim so it can call VNMS provisioning.

Rules:

- Do not log QR payloads.
- Redact secrets in errors and audit logs.
- Store raw claim secrets only if strictly needed and only encrypted with short
  expiry.
- Prefer storing hashes or opaque enrollment references.
- After VNMS provisioning succeeds, do not retain raw per-device keys in VSRV.
- VNMS is the long-term device key holder.

## Data Protection

Protected data includes:

- Movement and presence history.
- Household membership and location labels.
- `device_id` once linked to a household.
- IMEI/IMSI/ICCID and SIM/modem metadata.
- Push tokens.
- Payment/customer references.

Rules:

- Avoid sensitive data in logs.
- Avoid raw `device_id` in mobile URLs and analytics.
- Encrypt backups.
- Separate production and test data.
- Scope operator queries.
- Design for user data export and deletion.
- Keep raw/near-raw movement retention bounded.
- Treat AI/health inference as a separate consented data regime.

## Logging

Log:

- Request ID.
- User ID where authenticated.
- Household ID where relevant.
- Device binding ID, not raw `device_id`, in normal product logs.
- Error code, not sensitive payload.

Do not log:

- Passwords.
- Session/refresh tokens.
- QR secrets/device keys.
- Full push tokens.
- Payment card data.
- Raw movement payloads.

## Audit Events

Audit security-sensitive and product-sensitive actions:

- Signup and email verification.
- Login failures beyond threshold.
- Password reset request/complete.
- Session revocation.
- Household/member changes.
- Device claim, transfer, removal.
- VNMS provision/unprovision calls.
- Monitored-window updates.
- Alert rule and notification preference changes.
- Subscription activation/cancel.
- Payment webhook state changes.
- Support/admin reads and writes.

Audit records should be immutable from application code. If deletion is required
for privacy law, use a controlled retention/anonymization process.

## Rate Limiting and Abuse Controls

Rate-limit:

- Signup.
- Login.
- Password reset.
- Email verification.
- QR claim attempts.
- Payment session creation.
- Push token registration.

Use per-IP and per-account limits. Add device/household limits for claim and
configuration endpoints.

## Admin and Support Security

Before production:

- MFA required for admin/support accounts.
- Separate admin/support roles from normal users.
- Support access must be scoped to a ticket or time window where possible.
- Audit all support reads.
- Avoid exposing raw movement unless necessary for a support task.
- Restrict admin APIs by network where practical.

## Incident Readiness

Before real customer data:

- Document secret rotation.
- Test database restore.
- Define contact path for payment provider incidents.
- Define push credential rotation.
- Define how to revoke all mobile sessions.
- Define how to deactivate a lost/stolen device through VSRV and VNMS.
