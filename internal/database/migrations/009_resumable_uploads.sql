ALTER TABLE users ADD COLUMN upload_quota INTEGER NOT NULL DEFAULT 0;
CREATE TABLE upload_sessions (
 id TEXT PRIMARY KEY,
 owner_id INTEGER NOT NULL REFERENCES users(id),
 client_key TEXT NOT NULL,
 filename TEXT NOT NULL,
 folder TEXT NOT NULL,
 file_size INTEGER NOT NULL,
 sha256 TEXT NOT NULL,
 keep_copy INTEGER NOT NULL DEFAULT 0,
 received INTEGER NOT NULL DEFAULT 0,
 state TEXT NOT NULL DEFAULT 'uploading',
 asset_id INTEGER REFERENCES assets(id),
 updated_at INTEGER NOT NULL,
 UNIQUE(owner_id,client_key)
);
CREATE INDEX upload_sessions_owner ON upload_sessions(owner_id,state);
CREATE INDEX assets_upload_digest ON assets(library_id,file_size,content_hash);
CREATE TABLE upload_quarantine (
 path TEXT PRIMARY KEY,
 original_path TEXT NOT NULL,
 owner_id INTEGER NOT NULL REFERENCES users(id),
 file_size INTEGER NOT NULL,
 created_at INTEGER NOT NULL
);
