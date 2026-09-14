CREATE TABLE users (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 username TEXT NOT NULL COLLATE NOCASE UNIQUE,
 display_name TEXT NOT NULL,
 password_hash TEXT NOT NULL,
 role TEXT NOT NULL CHECK(role IN ('admin','member')),
 enabled INTEGER NOT NULL DEFAULT 1,
 must_change_password INTEGER NOT NULL DEFAULT 0,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
ALTER TABLE sessions ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE CASCADE;
DELETE FROM sessions;
CREATE TABLE folder_grants (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 library_id TEXT NOT NULL REFERENCES libraries(id),
 folder TEXT NOT NULL DEFAULT '',
 label TEXT NOT NULL,
 UNIQUE(user_id,library_id,folder)
);
CREATE TABLE user_favorites (
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 asset_id INTEGER NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
 PRIMARY KEY(user_id,asset_id)
);
ALTER TABLE albums ADD COLUMN owner_id INTEGER REFERENCES users(id);
ALTER TABLE albums ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE album_assets ADD COLUMN added_by INTEGER REFERENCES users(id);
CREATE TABLE album_members (
 album_id INTEGER NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 role TEXT NOT NULL CHECK(role IN ('viewer','contributor')),
 PRIMARY KEY(album_id,user_id)
);
CREATE INDEX album_members_user ON album_members(user_id,album_id);
CREATE INDEX albums_owner ON albums(owner_id,id);
