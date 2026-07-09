-- VSRV initial schema. Supports multi-household, multi-caregiver, and multi-device
-- from the start. Application code migrates from in-memory to PostgreSQL in Phase 2.

CREATE TABLE IF NOT EXISTS users (
    id                  TEXT PRIMARY KEY,
    email               TEXT NOT NULL UNIQUE,
    email_verified_at   TIMESTAMPTZ,
    phone               TEXT,
    phone_verified_at   TIMESTAMPTZ,
    display_name        TEXT NOT NULL,
    password_hash       TEXT NOT NULL,
    timezone            TEXT,
    disabled_at         TIMESTAMPTZ,
    last_login_at       TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
    id                      TEXT PRIMARY KEY,
    user_id                 TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash      TEXT NOT NULL UNIQUE,
    rotated_from_session_id TEXT REFERENCES sessions(id),
    device_label            TEXT,
    platform                TEXT,
    app_version             TEXT,
    last_seen_at            TIMESTAMPTZ,
    revoked_at              TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions(user_id);

CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS households (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    timezone            TEXT NOT NULL DEFAULT 'Europe/Helsinki',
    country             TEXT,
    created_by_user_id  TEXT NOT NULL REFERENCES users(id),
    archived_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS household_members (
    household_id    TEXT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'caregiver', 'member')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, user_id)
);

CREATE INDEX IF NOT EXISTS household_members_user_id_idx ON household_members(user_id);

CREATE TABLE IF NOT EXISTS household_invites (
    id              TEXT PRIMARY KEY,
    household_id    TEXT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    email           TEXT NOT NULL,
    role            TEXT NOT NULL CHECK (role IN ('admin', 'caregiver', 'member')),
    token_hash      TEXT NOT NULL UNIQUE,
    invited_by      TEXT NOT NULL REFERENCES users(id),
    expires_at      TIMESTAMPTZ NOT NULL,
    accepted_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS household_invites_household_id_idx ON household_invites(household_id);

CREATE TABLE IF NOT EXISTS device_bindings (
    id                      TEXT PRIMARY KEY,
    household_id            TEXT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    device_id               TEXT NOT NULL,
    display_name            TEXT NOT NULL,
    room_or_location_label  TEXT,
    claim_status            TEXT NOT NULL DEFAULT 'enrolled',
    claimed_by_user_id      TEXT REFERENCES users(id),
    removed_at              TIMESTAMPTZ,
    removed_reason          TEXT,
    vnms_lifecycle_cache    TEXT,
    last_vnms_sync_at       TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS device_bindings_active_device_id_idx
    ON device_bindings(device_id)
    WHERE removed_at IS NULL;

CREATE INDEX IF NOT EXISTS device_bindings_household_id_idx ON device_bindings(household_id);

CREATE TABLE IF NOT EXISTS monitored_window_intent (
    id                          TEXT PRIMARY KEY,
    device_binding_id           TEXT NOT NULL UNIQUE REFERENCES device_bindings(id) ON DELETE CASCADE,
    timezone                    TEXT NOT NULL,
    windows_json                JSONB NOT NULL DEFAULT '[]'::jsonb,
    enabled                     BOOLEAN NOT NULL DEFAULT true,
    desired_version             INTEGER NOT NULL DEFAULT 1,
    last_sent_to_vnms_at        TIMESTAMPTZ,
    last_delivered_by_vnms_at   TIMESTAMPTZ,
    delivery_state              TEXT NOT NULL DEFAULT 'not_configured',
    created_by_user_id          TEXT REFERENCES users(id),
    updated_by_user_id          TEXT REFERENCES users(id),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id                          TEXT PRIMARY KEY,
    household_id                TEXT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    device_binding_id           TEXT NOT NULL UNIQUE REFERENCES device_bindings(id) ON DELETE CASCADE,
    status                      TEXT NOT NULL DEFAULT 'none',
    service_status              TEXT NOT NULL DEFAULT 'service_limited',
    plan_code                   TEXT NOT NULL DEFAULT 'device_monitoring_monthly',
    payment_provider            TEXT,
    provider_customer_id        TEXT,
    provider_subscription_id    TEXT,
    campaign_code               TEXT,
    current_period_start        TIMESTAMPTZ,
    current_period_end          TIMESTAMPTZ,
    trial_ends_at               TIMESTAMPTZ,
    cancel_at                   TIMESTAMPTZ,
    cancelled_at                TIMESTAMPTZ,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS alert_rules (
    id                  TEXT PRIMARY KEY,
    device_binding_id   TEXT NOT NULL REFERENCES device_bindings(id) ON DELETE CASCADE,
    type                TEXT NOT NULL,
    enabled             BOOLEAN NOT NULL DEFAULT true,
    severity            TEXT NOT NULL DEFAULT 'info',
    quiet_hours_start   TEXT,
    quiet_hours_end     TEXT,
    timezone            TEXT,
    channels_json       JSONB NOT NULL DEFAULT '["push"]'::jsonb,
    created_by_user_id  TEXT REFERENCES users(id),
    updated_by_user_id  TEXT REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS alert_rules_device_binding_id_idx ON alert_rules(device_binding_id);

CREATE TABLE IF NOT EXISTS alerts (
    id                  TEXT PRIMARY KEY,
    device_binding_id   TEXT NOT NULL REFERENCES device_bindings(id) ON DELETE CASCADE,
    rule_id             TEXT REFERENCES alert_rules(id) ON DELETE SET NULL,
    type                TEXT NOT NULL,
    severity            TEXT NOT NULL,
    status              TEXT NOT NULL,
    title               TEXT NOT NULL,
    body                TEXT NOT NULL,
    source_event_id     TEXT,
    first_seen_at       TIMESTAMPTZ NOT NULL,
    last_seen_at        TIMESTAMPTZ NOT NULL,
    resolved_at         TIMESTAMPTZ,
    acknowledged_at     TIMESTAMPTZ,
    acknowledged_by     TEXT REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS alerts_device_binding_id_idx ON alerts(device_binding_id);

CREATE TABLE IF NOT EXISTS push_tokens (
    id                  TEXT PRIMARY KEY,
    user_id             TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id          TEXT REFERENCES sessions(id) ON DELETE SET NULL,
    platform            TEXT NOT NULL,
    token_hash          TEXT NOT NULL,
    token_encrypted     BYTEA,
    environment         TEXT NOT NULL DEFAULT 'production',
    enabled             BOOLEAN NOT NULL DEFAULT true,
    last_registered_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_delivery_error TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS push_tokens_user_token_hash_idx
    ON push_tokens(user_id, token_hash);

CREATE TABLE IF NOT EXISTS notification_deliveries (
    id              BIGSERIAL PRIMARY KEY,
    user_id         TEXT REFERENCES users(id) ON DELETE SET NULL,
    alert_id        TEXT REFERENCES alerts(id) ON DELETE SET NULL,
    channel         TEXT NOT NULL,
    destination     TEXT,
    status          TEXT NOT NULL,
    provider_ref    TEXT,
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at    TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS vnms_event_cursor (
    id          INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    cursor_id   BIGINT NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO vnms_event_cursor (id, cursor_id)
VALUES (1, 0)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS processed_vnms_events (
    event_id        TEXT PRIMARY KEY,
    event_type      TEXT NOT NULL,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS system_config (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO system_config (key, value)
VALUES
    ('offline_threshold_hours', '3'),
    ('default_trial_days', '30')
ON CONFLICT (key) DO NOTHING;

CREATE TABLE IF NOT EXISTS audit_log (
    id              BIGSERIAL PRIMARY KEY,
    actor_user_id   TEXT REFERENCES users(id) ON DELETE SET NULL,
    action          TEXT NOT NULL,
    resource_type   TEXT,
    resource_id     TEXT,
    metadata_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS audit_log_created_at_idx ON audit_log(created_at);
