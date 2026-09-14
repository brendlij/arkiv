CREATE VIRTUAL TABLE assets_fts USING fts5(filename,folder,camera_make,camera_model,lens,taken_at,focal_length,content='assets',content_rowid='id');
CREATE TRIGGER assets_ai AFTER INSERT ON assets BEGIN
 INSERT INTO assets_fts(rowid,filename,folder,camera_make,camera_model,lens,taken_at,focal_length) VALUES(new.id,new.filename,new.folder,new.camera_make,new.camera_model,new.lens,new.taken_at,new.focal_length);
END;
CREATE TRIGGER assets_ad AFTER DELETE ON assets BEGIN
 INSERT INTO assets_fts(assets_fts,rowid,filename,folder,camera_make,camera_model,lens,taken_at,focal_length) VALUES('delete',old.id,old.filename,old.folder,old.camera_make,old.camera_model,old.lens,old.taken_at,old.focal_length);
END;
CREATE TRIGGER assets_au AFTER UPDATE OF filename,folder,camera_make,camera_model,lens,taken_at,focal_length ON assets BEGIN
 INSERT INTO assets_fts(assets_fts,rowid,filename,folder,camera_make,camera_model,lens,taken_at,focal_length) VALUES('delete',old.id,old.filename,old.folder,old.camera_make,old.camera_model,old.lens,old.taken_at,old.focal_length);
 INSERT INTO assets_fts(rowid,filename,folder,camera_make,camera_model,lens,taken_at,focal_length) VALUES(new.id,new.filename,new.folder,new.camera_make,new.camera_model,new.lens,new.taken_at,new.focal_length);
END;
INSERT INTO assets_fts(assets_fts) VALUES('rebuild');
