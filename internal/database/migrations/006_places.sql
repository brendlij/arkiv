ALTER TABLE assets ADD COLUMN place_id TEXT NOT NULL DEFAULT '';
ALTER TABLE assets ADD COLUMN place_name TEXT NOT NULL DEFAULT '';
ALTER TABLE assets ADD COLUMN place_checked INTEGER NOT NULL DEFAULT 0;
CREATE INDEX assets_location ON assets(latitude,longitude);
CREATE INDEX assets_place ON assets(place_id,taken_at DESC,id DESC);
DROP TRIGGER assets_ai;
DROP TRIGGER assets_ad;
DROP TRIGGER assets_au;
DROP TABLE assets_fts;
CREATE VIRTUAL TABLE assets_fts USING fts5(filename,folder,camera_make,camera_model,lens,taken_at,focal_length,place_name,content='assets',content_rowid='id');
CREATE TRIGGER assets_ai AFTER INSERT ON assets BEGIN
 INSERT INTO assets_fts(rowid,filename,folder,camera_make,camera_model,lens,taken_at,focal_length,place_name) VALUES(new.id,new.filename,new.folder,new.camera_make,new.camera_model,new.lens,new.taken_at,new.focal_length,new.place_name);
END;
CREATE TRIGGER assets_ad AFTER DELETE ON assets BEGIN
 INSERT INTO assets_fts(assets_fts,rowid,filename,folder,camera_make,camera_model,lens,taken_at,focal_length,place_name) VALUES('delete',old.id,old.filename,old.folder,old.camera_make,old.camera_model,old.lens,old.taken_at,old.focal_length,old.place_name);
END;
CREATE TRIGGER assets_au AFTER UPDATE OF filename,folder,camera_make,camera_model,lens,taken_at,focal_length,place_name ON assets BEGIN
 INSERT INTO assets_fts(assets_fts,rowid,filename,folder,camera_make,camera_model,lens,taken_at,focal_length,place_name) VALUES('delete',old.id,old.filename,old.folder,old.camera_make,old.camera_model,old.lens,old.taken_at,old.focal_length,old.place_name);
 INSERT INTO assets_fts(rowid,filename,folder,camera_make,camera_model,lens,taken_at,focal_length,place_name) VALUES(new.id,new.filename,new.folder,new.camera_make,new.camera_model,new.lens,new.taken_at,new.focal_length,new.place_name);
END;
INSERT INTO assets_fts(assets_fts) VALUES('rebuild');
