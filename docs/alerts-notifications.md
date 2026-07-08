# Alerts and Notifications

## Purpose

VSRV owns alert policy and push notification delivery. VNMS emits authenticated
device facts; VSRV decides whether those facts become user-visible alerts,
timeline entries, support signals, or push notifications.

## Source Facts

VNMS event/fact sources include:

- `monitored_window.movement_detected`
- `monitored_window.no_movement_detected`
- `device_status.received`
- `device_info.received`
- `device.policy_delivered`
- `device.lifecycle_changed`
- future low-battery or missed-contact facts

VSRV can also derive facts from:

- VNMS last-contact state.
- Expected contact cadence.
- Subscription/service state.
- User alert rules.
- Device binding state.

## Initial Alert Types

Product alert types:

- `movement_detected`
- `no_movement_detected`
- `device_offline`
- `low_battery`
- `poor_coverage`
- `policy_not_delivered`
- `device_removed_or_revoked`

MVP can start with monitored-window movement/no-movement and device offline.

## Alert Rule Model

Each alert rule belongs to a device binding or household.

Suggested fields:

- `id`
- `device_binding_id`
- `type`
- `enabled`
- `severity`
- `quiet_hours_start`
- `quiet_hours_end`
- `timezone`
- `channels`
- `recipient_policy`
- `created_by_user_id`
- `updated_by_user_id`

Recommended behavior:

- Rule evaluation is idempotent per source event.
- Quiet hours suppress push delivery, not necessarily alert creation.
- Alert creation and push delivery are separate steps.
- Store active/resolved state so the app can show alerts even if push fails.

## Alert Instance Model

Suggested fields:

- `id`
- `device_binding_id`
- `rule_id`
- `source_event_type`
- `source_event_id`
- `type`
- `severity`
- `status`
- `title`
- `body`
- `first_seen_at`
- `last_seen_at`
- `resolved_at`
- `acknowledged_at`
- `acknowledged_by_user_id`

Statuses:

- `active`
- `acknowledged`
- `resolved`
- `suppressed`

## Event Processing Flow

```text
VNMS event cursor
  -> VSRV event consumer
  -> deduplicate by VNMS event ID/idempotency key
  -> load device binding and household policy
  -> evaluate alert rules
  -> create/update AlertInstance
  -> enqueue notification jobs
  -> update mobile projections/timeline
```

Notification delivery must be asynchronous. Do not block VNMS event cursor
progress on APNs/FCM calls indefinitely; use an internal outbox/job table.

## Push Notification Flow

```text
AlertInstance created
  -> notification job inserted
  -> load eligible users and push tokens
  -> apply quiet hours and recipient rules
  -> send via APNs/FCM
  -> record delivery result
```

VSRV stores:

- Push token registration.
- Delivery attempts.
- Last provider error.
- Token disabled state when provider says token is invalid.

## Notification Content

Keep push content concise and privacy-aware.

Examples:

- "Movement detected in Living room."
- "No movement detected during the monitored window."
- "Vigelo device has not checked in recently."
- "Battery may need attention soon."

Avoid:

- Raw device IDs.
- IMEI/IMSI/ICCID.
- Detailed movement history.
- Health/diagnostic conclusions unless explicitly designed and consented.

## Offline and Missed Contact Alerts

VNMS knows last contact and expected send floor. VSRV can derive user-facing
offline state using:

- `last_contact_at`
- monitored-window policy
- max-silence rule
- battery survival allowances
- subscription/service status

Initial heuristic:

- `online`: last contact within expected recent window.
- `delayed`: slightly beyond expected contact.
- `offline`: beyond allowed grace period.

Make thresholds configurable. Avoid alarming users too aggressively during early
NB-IoT/operator testing.

## Low Battery Alerts

VNMS receives voltage through status uplinks and stores daily voltage samples.
VSRV should define product thresholds later after battery behavior is measured.

Initial direction:

- Store latest voltage in product view.
- Display voltage as volts, for example `3.000 V`.
- Do not send user low-battery push until thresholds are validated.
- Use fleet analysis to tune thresholds by battery chemistry and device revision.

## Monitored Window Alerts

VNMS emits monitored-window facts. VSRV applies user preference:

- Alert on movement.
- Alert on no movement.
- Alert only during selected windows.
- Suppress during quiet hours.
- Notify selected household members/caregivers.

No-movement alerts should be emitted only after the monitored window is complete
and VNMS has enough observed data to support the fact.

## Timeline

Mobile timeline should be a product projection:

- movement detected in monitored window
- no movement detected in monitored window
- device status updated
- monitored hours changed
- device came online/offline
- alert acknowledged/resolved

Do not expose raw VNMS event payloads directly.

## Open Decisions

- Exact alert defaults.
- Quiet-hour UX.
- Recipient/escalation model.
- Offline thresholds.
- Battery thresholds.
- Whether SMS/email channels are needed after push.
- Caregiver role semantics.
