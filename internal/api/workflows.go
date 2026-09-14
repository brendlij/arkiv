package api

import (
	"encoding/json"
	"gallery/internal/access"
	"gallery/internal/library"
	"net/http"
	"os"
	"path"
	"strings"
)

func (s *Server) registerWorkflows(m *http.ServeMux) {
	m.HandleFunc("GET /api/uploads", s.uploadOptions)
	m.HandleFunc("POST /api/uploads", s.upload)
	m.HandleFunc("GET /api/map", s.mapPoints)
	m.HandleFunc("GET /api/places", s.placeList)
	m.HandleFunc("GET /api/albums/{id}/public-link", s.publicSettings)
	m.HandleFunc("POST /api/albums/{id}/public-link", s.publicSettings)
	m.HandleFunc("DELETE /api/albums/{id}/public-link", s.publicSettings)
	m.HandleFunc("GET /api/public/{token}", s.publicAlbum)
	m.HandleFunc("GET /api/public/{token}/assets/{id}/thumbnail/{size}", s.publicFile)
	m.HandleFunc("GET /api/public/{token}/assets/{id}/original", s.publicFile)
	m.HandleFunc("PATCH /api/albums/{id}", s.editAlbum)
	m.HandleFunc("DELETE /api/albums/{id}", s.editAlbum)
	m.HandleFunc("GET /api/albums/{id}/members", s.members)
	m.HandleFunc("POST /api/albums/{id}/members", s.members)
	m.HandleFunc("DELETE /api/albums/{id}/members/{userId}", s.members)
	m.HandleFunc("GET /api/me/folders", s.myFolders)
	m.HandleFunc("GET /api/admin/libraries", s.adminLibraries)
	m.HandleFunc("GET /api/admin/users/{userId}/folders", s.grants)
	m.HandleFunc("POST /api/admin/users/{userId}/folders", s.grants)
	m.HandleFunc("DELETE /api/admin/users/{userId}/folders/{grantId}", s.grants)
}
func (s *Server) adminLibraries(w http.ResponseWriter, r *http.Request) {
	if !access.Admin(w, r) {
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), "SELECT id,name FROM libraries WHERE enabled=1 ORDER BY name")
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var id, name string
		if e = rows.Scan(&id, &name); e != nil {
			s.fail(w, e)
			return
		}
		items = append(items, map[string]string{"id": id, "name": name})
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	send(w, map[string]any{"items": items})
}
func (s *Server) myFolders(w http.ResponseWriter, r *http.Request) {
	s.listGrants(w, r, access.Current(r).ID)
}
func (s *Server) listGrants(w http.ResponseWriter, r *http.Request, id int64) {
	rows, e := s.DB.QueryContext(r.Context(), `SELECT fg.id,fg.library_id,fg.folder,fg.label,(SELECT count(*) FROM assets a WHERE a.trashed_at=0 AND a.library_id=fg.library_id AND (fg.folder='' OR a.folder=fg.folder OR substr(a.folder,1,length(fg.folder)+1)=fg.folder||'/')) FROM folder_grants fg JOIN libraries l ON l.id=fg.library_id AND l.enabled=1 WHERE fg.user_id=? ORDER BY fg.label`, id)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var gid, count int64
		var lib, folder, label string
		if e = rows.Scan(&gid, &lib, &folder, &label, &count); e != nil {
			s.fail(w, e)
			return
		}
		items = append(items, map[string]any{"id": gid, "libraryId": lib, "path": folder, "name": label, "count": count})
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	send(w, map[string]any{"items": items, "hasMore": false})
}
func (s *Server) grants(w http.ResponseWriter, r *http.Request) {
	if !access.Admin(w, r) {
		return
	}
	id, e := idParam(r, "userId")
	if e != nil {
		http.Error(w, "invalid user", 400)
		return
	}
	if r.Method == "GET" {
		s.listGrants(w, r, id)
		return
	}
	if r.Method == "DELETE" {
		gid, e := idParam(r, "grantId")
		if e != nil {
			http.Error(w, "invalid grant", 400)
			return
		}
		_, e = s.DB.ExecContext(r.Context(), "DELETE FROM folder_grants WHERE id=? AND user_id=?", gid, id)
		if e != nil {
			s.fail(w, e)
			return
		}
		w.WriteHeader(204)
		return
	}
	var b struct {
		LibraryID string `json:"libraryId"`
		Folder    string `json:"folder"`
		Label     string `json:"label"`
	}
	if json.NewDecoder(r.Body).Decode(&b) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	b.Label = strings.TrimSpace(b.Label)
	if len(b.Folder) > 4096 || len([]rune(b.Label)) < 1 || len([]rune(b.Label)) > 120 || strings.ContainsAny(b.Folder, "\\:\x00") || strings.HasPrefix(b.Folder, "/") || (b.Folder != "" && (path.Clean(b.Folder) != b.Folder || b.Folder == "." || b.Folder == ".." || strings.HasPrefix(b.Folder, "../"))) {
		http.Error(w, "use a canonical relative folder and a label", 400)
		return
	}
	var root string
	if e = s.DB.QueryRowContext(r.Context(), "SELECT root FROM libraries WHERE id=? AND enabled=1", b.LibraryID).Scan(&root); e != nil {
		http.Error(w, "library unavailable", 400)
		return
	}
	if b.LibraryID == "arkiv-uploads" {
		var exists int
		if e = s.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM managed_folders WHERE path=?", b.Folder).Scan(&exists); e != nil || (exists == 0 && b.Folder != "") {
			http.Error(w, "Folder unavailable", 400)
			return
		}
	} else {
		rel := b.Folder
		if rel == "" {
			rel = "."
		}
		resolved, e := library.Resolve(root, rel)
		if e != nil {
			http.Error(w, "folder unavailable", 400)
			return
		}
		info, e := os.Stat(resolved)
		if e != nil || !info.IsDir() {
			http.Error(w, "folder unavailable", 400)
			return
		}
	}
	_, e = s.DB.ExecContext(r.Context(), "INSERT INTO folder_grants(user_id,library_id,folder,label) VALUES(?,?,?,?) ON CONFLICT(user_id,library_id,folder) DO UPDATE SET label=excluded.label", id, b.LibraryID, b.Folder, b.Label)
	if e != nil {
		http.Error(w, "folder assignment failed", 400)
		return
	}
	w.WriteHeader(204)
}
