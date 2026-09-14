package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"gallery/internal/access"
	"net/http"
	"strings"
)

func uploadPath(uid int64, relative string) (string, error) {
	if len(relative) > 240 || strings.ContainsAny(relative, "\\:\x00\r\n") {
		return "", fmt.Errorf("Invalid folder path")
	}
	if relative != "" {
		for _, part := range strings.Split(relative, "/") {
			if part == "" || part == "." || part == ".." || strings.TrimSpace(part) != part {
				return "", fmt.Errorf("Use folder names separated by /, without empty names or ..")
			}
		}
	}
	root := fmt.Sprintf("user-%d", uid)
	if relative != "" {
		root += "/" + relative
	}
	return root, nil
}
func ensureUploadFolder(ctx context.Context, tx *sql.Tx, uid int64, path string) error {
	parts := strings.Split(path, "/")
	for i := range parts {
		if _, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO managed_folders(path,owner_id) VALUES(?,?)", strings.Join(parts[:i+1], "/"), uid); e != nil {
			return e
		}
	}
	_, e := tx.ExecContext(ctx, "INSERT INTO folder_grants(user_id,library_id,folder,label) VALUES(?,'arkiv-uploads',?,'My uploads') ON CONFLICT(user_id,library_id,folder) DO NOTHING", uid, parts[0])
	return e
}
func (s *Server) manageFolder(w http.ResponseWriter, r *http.Request) {
	u := access.Current(r)
	if u.ID < 1 || u.PublicAlbum != 0 {
		http.Error(w, "Sign in required", 401)
		return
	}
	var b struct {
		Action   string `json:"action"`
		Path     string `json:"path"`
		Target   string `json:"target"`
		Username string `json:"username"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&b) != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	source, e := uploadPath(u.ID, b.Path)
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	uploadLock.Lock()
	defer uploadLock.Unlock()
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	var enabled int
	if e = tx.QueryRowContext(r.Context(), "SELECT enabled FROM libraries WHERE id='arkiv-uploads'").Scan(&enabled); e != nil || enabled != 1 {
		http.Error(w, "Uploads unavailable", 409)
		return
	}
	root, _ := uploadPath(u.ID, "")
	if e = ensureUploadFolder(r.Context(), tx, u.ID, root); e != nil {
		s.fail(w, e)
		return
	}
	var exists int
	if b.Action != "create" {
		if e = tx.QueryRowContext(r.Context(), "SELECT count(*) FROM managed_folders WHERE path=? AND owner_id=?", source, u.ID).Scan(&exists); e != nil {
			s.fail(w, e)
			return
		}
		if exists == 0 {
			http.Error(w, "Folder not found", 404)
			return
		}
	}
	switch b.Action {
	case "create":
		if e = tx.QueryRowContext(r.Context(), "SELECT count(*) FROM managed_folders WHERE path=?", source).Scan(&exists); e != nil {
			s.fail(w, e)
			return
		}
		if exists > 0 {
			http.Error(w, "Folder already exists", 409)
			return
		}
		e = ensureUploadFolder(r.Context(), tx, u.ID, source)
	case "move":
		var transfers int
		if e = tx.QueryRowContext(r.Context(), "SELECT count(*) FROM upload_sessions WHERE owner_id=? AND state IN ('uploading','committing') AND (folder=? OR substr(folder,1,length(?)+1)=?||'/')", u.ID, source, source, source).Scan(&transfers); e != nil {
			s.fail(w, e)
			return
		}
		if transfers > 0 {
			http.Error(w, "Finish or discard paused uploads in this folder before moving or renaming it", 409)
			return
		}
		target, err := uploadPath(u.ID, b.Target)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if source == root || target == root || target == source || strings.HasPrefix(target, source+"/") {
			http.Error(w, "Choose a different destination outside this folder", 400)
			return
		}
		if e = tx.QueryRowContext(r.Context(), "SELECT count(*) FROM managed_folders WHERE path=?", target).Scan(&exists); e != nil {
			s.fail(w, e)
			return
		}
		if exists > 0 {
			http.Error(w, "Destination folder already exists", 409)
			return
		}
		parent := target[:strings.LastIndex(target, "/")]
		if e = ensureUploadFolder(r.Context(), tx, u.ID, parent); e != nil {
			s.fail(w, e)
			return
		}
		// Re-key grants with the subtree so explicitly shared folders retain access.
		for _, q := range []string{
			"UPDATE assets SET folder=?||substr(folder,length(?)+1) WHERE library_id='arkiv-uploads' AND (folder=? OR substr(folder,1,length(?)+1)=?||'/')",
			"UPDATE folder_grants SET folder=?||substr(folder,length(?)+1) WHERE library_id='arkiv-uploads' AND (folder=? OR substr(folder,1,length(?)+1)=?||'/')",
			"UPDATE managed_folders SET path=?||substr(path,length(?)+1) WHERE path=? OR substr(path,1,length(?)+1)=?||'/'",
		} {
			if _, e = tx.ExecContext(r.Context(), q, target, source, source, source, source); e != nil {
				s.fail(w, e)
				return
			}
		}
		source = target
	case "share", "revoke":
		var recipient int64
		if e = tx.QueryRowContext(r.Context(), "SELECT id FROM users WHERE username=? AND (enabled=1 OR ?='revoke')", strings.TrimSpace(b.Username), b.Action).Scan(&recipient); e != nil {
			http.Error(w, "Choose an existing enabled username", 400)
			return
		}
		if recipient == u.ID {
			http.Error(w, "You already own this folder", 400)
			return
		}
		if b.Action == "share" {
			label := "Shared by " + u.DisplayName + ": " + strings.TrimPrefix(source, root+"/")
			if source == root {
				label = "Uploads shared by " + u.DisplayName
			}
			_, e = tx.ExecContext(r.Context(), "INSERT INTO folder_grants(user_id,library_id,folder,label) VALUES(?,'arkiv-uploads',?,?) ON CONFLICT(user_id,library_id,folder) DO UPDATE SET label=excluded.label", recipient, source, label)
		} else {
			_, e = tx.ExecContext(r.Context(), "DELETE FROM folder_grants WHERE user_id=? AND library_id='arkiv-uploads' AND folder=?", recipient, source)
		}
	default:
		http.Error(w, "Unknown folder action", 400)
		return
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		s.fail(w, e)
		return
	}
	send(w, map[string]string{"path": source})
}

func (s *Server) managedFolderList(w http.ResponseWriter, r *http.Request, parent string) {
	u := access.Current(r)
	// Folder access uses the same inherited grants, including empty folders.
	predicate := "EXISTS(SELECT 1 FROM folder_grants fg WHERE fg.user_id=? AND fg.library_id='arkiv-uploads' AND (fg.folder='' OR f.path=fg.folder OR substr(f.path,1,length(fg.folder)+1)=fg.folder||'/'))"
	if u.Role == "admin" {
		predicate = "? > 0"
	}
	rows, e := s.DB.QueryContext(r.Context(), "SELECT f.path,(SELECT count(*) FROM assets a WHERE a.library_id='arkiv-uploads' AND a.trashed_at=0 AND (a.folder=f.path OR substr(a.folder,1,length(f.path)+1)=f.path||'/')) FROM managed_folders f WHERE EXISTS(SELECT 1 FROM libraries WHERE id='arkiv-uploads' AND enabled=1) AND "+predicate+" AND substr(f.path,1,length(?))=? AND instr(substr(f.path,length(?)+1),'/')=0 AND f.path<>? ORDER BY f.path", u.ID, folderPrefix(parent), folderPrefix(parent), folderPrefix(parent), parent)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var path string
		var count int
		if e = rows.Scan(&path, &count); e != nil {
			s.fail(w, e)
			return
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if path == fmt.Sprintf("user-%d", u.ID) {
			name = "My uploads"
		}
		items = append(items, map[string]any{"libraryId": "arkiv-uploads", "path": path, "name": name, "count": count})
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	send(w, map[string]any{"items": items, "hasMore": false})
}
func folderPrefix(path string) string {
	if path == "" {
		return ""
	}
	return path + "/"
}

func (s *Server) folderSharing(w http.ResponseWriter, r *http.Request) {
	u := access.Current(r)
	if u.ID < 1 || u.PublicAlbum != 0 {
		http.Error(w, "Sign in required", 401)
		return
	}
	path, e := uploadPath(u.ID, r.URL.Query().Get("path"))
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), "SELECT u.username,u.display_name,fg.folder FROM folder_grants fg JOIN users u ON u.id=fg.user_id WHERE fg.library_id='arkiv-uploads' AND u.id<>? AND (fg.folder='' OR fg.folder=? OR substr(?,1,length(fg.folder)+1)=fg.folder||'/') ORDER BY u.display_name", u.ID, path, path)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var username, name, source string
		if e = rows.Scan(&username, &name, &source); e != nil {
			s.fail(w, e)
			return
		}
		items = append(items, map[string]any{"username": username, "name": name, "inherited": source != path, "source": source})
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	send(w, items)
}
