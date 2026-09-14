CREATE TABLE managed_folders (
 path TEXT PRIMARY KEY,
 owner_id INTEGER NOT NULL REFERENCES users(id)
);
INSERT OR IGNORE INTO managed_folders SELECT 'user-'||id,id FROM users;
INSERT OR IGNORE INTO managed_folders SELECT a.folder,u.id FROM assets a JOIN users u ON substr(a.relative_path,1,length('user-'||u.id||'/'))='user-'||u.id||'/' WHERE a.library_id='arkiv-uploads';
WITH RECURSIVE prefixes(path,rest,owner_id) AS (
 SELECT '',path||'/',owner_id FROM managed_folders
 UNION ALL SELECT path||substr(rest,1,instr(rest,'/')),substr(rest,instr(rest,'/')+1),owner_id FROM prefixes WHERE instr(rest,'/')>0
)
INSERT OR IGNORE INTO managed_folders SELECT rtrim(path,'/'),owner_id FROM prefixes WHERE path<>'';
