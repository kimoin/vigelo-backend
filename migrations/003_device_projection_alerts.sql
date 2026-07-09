-- Device projection cache for offline detection and alert idempotency.

ALTER TABLE device_bindings
    ADD COLUMN IF NOT EXISTS last_contact_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_voltage_mv INTEGER;

CREATE UNIQUE INDEX IF NOT EXISTS alerts_source_event_id_idx
    ON alerts(source_event_id)
    WHERE source_event_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS alerts_active_type_per_binding_idx
    ON alerts(device_binding_id, type)
    WHERE status = 'active';

CREATE UNIQUE INDEX IF NOT EXISTS alert_rules_binding_type_idx
    ON alert_rules(device_binding_id, type);
