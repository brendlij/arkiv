ALTER TABLE assets ADD COLUMN trashed_at INTEGER NOT NULL DEFAULT 0;
CREATE INDEX assets_trash ON assets(trashed_at);
