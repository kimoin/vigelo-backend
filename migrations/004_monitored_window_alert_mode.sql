-- User chooses movement OR no-movement alerts for monitored windows (not both).

ALTER TABLE monitored_window_intent
    ADD COLUMN IF NOT EXISTS alert_mode TEXT NOT NULL DEFAULT 'no_movement_detected';
