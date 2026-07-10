-- Human-readable audit messages for admin log view.
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS message TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS audit_log_action_idx ON audit_log(action);
