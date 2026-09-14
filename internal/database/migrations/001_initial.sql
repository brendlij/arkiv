CREATE TABLE libraries(id TEXT PRIMARY KEY,name TEXT NOT NULL,root TEXT NOT NULL,enabled INTEGER NOT NULL);
CREATE TABLE assets(
 id INTEGER PRIMARY KEY AUTOINCREMENT, library_id TEXT NOT NULL REFERENCES libraries(id),
 relative_path TEXT NOT NULL, folder TEXT NOT NULL, filename TEXT NOT NULL, extension TEXT NOT NULL,
 mime_type TEXT NOT NULL, media_type TEXT NOT NULL, file_size INTEGER NOT NULL, modified_at INTEGER NOT NULL,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, taken_at TEXT NOT NULL, indexed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 width INTEGER NOT NULL DEFAULT 0,height INTEGER NOT NULL DEFAULT 0,orientation INTEGER NOT NULL DEFAULT 1,
 camera_make TEXT NOT NULL DEFAULT '',camera_model TEXT NOT NULL DEFAULT '',lens TEXT NOT NULL DEFAULT '',
 focal_length TEXT NOT NULL DEFAULT '',aperture TEXT NOT NULL DEFAULT '',exposure_time TEXT NOT NULL DEFAULT '',iso TEXT NOT NULL DEFAULT '',
 latitude REAL,longitude REAL,duration REAL NOT NULL DEFAULT 0,video_codec TEXT NOT NULL DEFAULT '',audio_codec TEXT NOT NULL DEFAULT '',
 favorite INTEGER NOT NULL DEFAULT 0,content_hash TEXT,preview_key TEXT NOT NULL DEFAULT '',
 preview_status TEXT NOT NULL DEFAULT 'pending',metadata_status TEXT NOT NULL DEFAULT 'pending',error TEXT NOT NULL DEFAULT '',
 generation INTEGER NOT NULL DEFAULT 1,seen_scan TEXT NOT NULL,UNIQUE(library_id,relative_path)
);
CREATE INDEX assets_timeline ON assets(taken_at DESC,id DESC);
CREATE INDEX assets_type ON assets(media_type,taken_at DESC,id DESC);
CREATE INDEX assets_favorite ON assets(favorite,taken_at DESC,id DESC);
CREATE INDEX assets_camera ON assets(camera_model);
CREATE INDEX assets_lens ON assets(lens);
CREATE INDEX assets_folder ON assets(library_id,folder);
CREATE TABLE jobs(asset_id INTEGER PRIMARY KEY REFERENCES assets(id) ON DELETE CASCADE,state TEXT NOT NULL DEFAULT 'pending',attempts INTEGER NOT NULL DEFAULT 0,next_at INTEGER NOT NULL DEFAULT 0);
CREATE INDEX jobs_ready ON jobs(state,next_at);
CREATE TABLE albums(id INTEGER PRIMARY KEY,name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 120),created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE album_assets(album_id INTEGER NOT NULL REFERENCES albums(id) ON DELETE CASCADE,asset_id INTEGER NOT NULL REFERENCES assets(id) ON DELETE CASCADE,PRIMARY KEY(album_id,asset_id));
CREATE TABLE sessions(token_hash TEXT PRIMARY KEY,expires_at INTEGER NOT NULL);
