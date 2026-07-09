-- Phase 2: session access tokens and expiry for bearer auth + refresh rotation.

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS access_token_hash TEXT,
    ADD COLUMN IF NOT EXISTS access_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS refresh_expires_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS sessions_access_token_hash_idx
    ON sessions(access_token_hash)
    WHERE access_token_hash IS NOT NULL;
