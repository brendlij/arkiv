ALTER TABLE sessions ADD COLUMN credential_scope TEXT NOT NULL DEFAULT '';
CREATE INDEX sessions_expiry ON sessions(expires_at);
