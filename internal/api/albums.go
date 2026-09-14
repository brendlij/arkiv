package api

import (
	"database/sql"
	"encoding/json"
	"gallery/internal/access"
	"net/http"
	"strconv"
	"strings"
)

type album struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Count       int    `json:"count"`
	OwnerID     int64  `json:"ownerId"`
	OwnerName   string `json:"ownerName"`
	Role        string `json:"role"`
}

func (s *Server) albumRole(r *http.Request, id int64) string {
	u := access.Current(r)
	var owner int64
	var role string
	e := s.DB.QueryRowContext(r.Context(), "SELECT owner_id FROM albums WHERE id=?", id).Scan(&owner)
	if e != nil {
		return ""
	}
	if u.ID == owner || u.Role == "admin" {
		return "owner"
	}
	s.DB.QueryRowContext(r.Context(), "SELECT role FROM album_members WHERE album_id=? AND user_id=? AND starts_at<=unixepoch() AND (expires_at=0 OR expires_at>unixepoch())", id, u.ID).Scan(&role)
	return role
}
func (s *Server) albumAllowed(w http.ResponseWriter, r *http.Request, id int64, edit bool) bool {
	role := s.albumRole(r, id)
	if role == "" {
		http.NotFound(w, r)
		return false
	}
	if edit && role == "viewer" {
		http.Error(w, "album is view only", 403)
		return false
	}
	return true
}
func (s *Server) albumQuery(r *http.Request) string {
	u := access.Current(r)
	return `SELECT al.id,al.name,al.description,count(a.id),al.owner_id,u.display_name,CASE WHEN al.owner_id=` + strconv.FormatInt(u.ID, 10) + ` OR '` + u.Role + `'='admin' THEN 'owner' ELSE coalesce((SELECT role FROM album_members WHERE album_id=al.id AND user_id=` + strconv.FormatInt(u.ID, 10) + `),'') END FROM albums al JOIN users u ON u.id=al.owner_id LEFT JOIN album_assets aa ON aa.album_id=al.id LEFT JOIN assets a ON a.id=aa.asset_id AND a.library_id IN(SELECT id FROM libraries WHERE enabled=1) AND ` + access.Visible(u, "a") + ` WHERE ` + access.Album(u, "al")
}
func readAlbum(row rowScanner) (album, error) {
	var a album
	e := row.Scan(&a.ID, &a.Name, &a.Description, &a.Count, &a.OwnerID, &a.OwnerName, &a.Role)
	return a, e
}
func (s *Server) albums(w http.ResponseWriter, r *http.Request) {
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		var e error
		offset, e = strconv.Atoi(v)
		if e != nil || offset < 0 {
			http.Error(w, "invalid offset", 400)
			return
		}
	}
	q := s.albumQuery(r)
	switch r.URL.Query().Get("scope") {
	case "owned":
		q += " AND al.owner_id=" + strconv.FormatInt(access.Current(r).ID, 10)
	case "shared":
		q += " AND al.owner_id<>" + strconv.FormatInt(access.Current(r).ID, 10) + " AND EXISTS(SELECT 1 FROM album_members WHERE album_id=al.id AND user_id=" + strconv.FormatInt(access.Current(r).ID, 10) + ")"
	}
	rows, e := s.DB.QueryContext(r.Context(), q+" GROUP BY al.id ORDER BY al.name,al.id LIMIT 201 OFFSET ?", offset)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	items := []album{}
	for rows.Next() {
		a, e := readAlbum(rows)
		if e != nil {
			s.fail(w, e)
			return
		}
		items = append(items, a)
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	more := len(items) > 200
	if more {
		items = items[:200]
	}
	send(w, map[string]any{"items": items, "hasMore": more})
}
func (s *Server) createAlbum(w http.ResponseWriter, r *http.Request) {
	var b album
	if json.NewDecoder(r.Body).Decode(&b) != nil || len([]rune(strings.TrimSpace(b.Name))) < 1 || len([]rune(b.Name)) > 120 || len(b.Description) > 2000 {
		http.Error(w, "album name must contain 1..120 characters", 400)
		return
	}
	u := access.Current(r)
	res, e := s.DB.ExecContext(r.Context(), "INSERT INTO albums(name,description,owner_id) VALUES(?,?,?)", strings.TrimSpace(b.Name), b.Description, u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	b.ID, e = res.LastInsertId()
	if e != nil {
		s.fail(w, e)
		return
	}
	b.OwnerID = u.ID
	b.OwnerName = u.DisplayName
	b.Role = "owner"
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	send(w, b)
}
func (s *Server) album(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r, "id")
	if e != nil {
		http.Error(w, "invalid ID", 400)
		return
	}
	a, e := readAlbum(s.DB.QueryRowContext(r.Context(), s.albumQuery(r)+" AND al.id=? GROUP BY al.id", id))
	if e == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	send(w, a)
}
func (s *Server) addAlbumAsset(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r, "id")
	if e != nil {
		http.Error(w, "invalid ID", 400)
		return
	}
	if !s.albumAllowed(w, r, id, true) {
		return
	}
	var b struct {
		AssetID int64 `json:"assetId"`
	}
	if json.NewDecoder(r.Body).Decode(&b) != nil || b.AssetID < 1 {
		http.Error(w, "assetId required", 400)
		return
	}
	u := access.Current(r)
	var n int
	if e = s.DB.QueryRowContext(r.Context(), "SELECT 1 FROM assets a JOIN libraries l ON l.id=a.library_id WHERE a.id=? AND l.enabled=1 AND "+access.Direct(u, "a"), b.AssetID).Scan(&n); e == sql.ErrNoRows {
		http.Error(w, "only photos from your assigned folders can be added", 403)
		return
	} else if e != nil {
		s.fail(w, e)
		return
	}
	_, e = s.DB.ExecContext(r.Context(), "INSERT INTO album_assets(album_id,asset_id,added_by) VALUES(?,?,?) ON CONFLICT DO NOTHING", id, b.AssetID, u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) removeAlbumAsset(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r, "id")
	assetID, e2 := idParam(r, "assetId")
	if e != nil || e2 != nil {
		http.Error(w, "invalid ID", 400)
		return
	}
	if !s.albumAllowed(w, r, id, true) {
		return
	}
	q := "DELETE FROM album_assets WHERE album_id=? AND asset_id=?"
	if s.albumRole(r, id) != "owner" {
		var n int
		if e = s.DB.QueryRowContext(r.Context(), "SELECT 1 FROM album_assets WHERE album_id=? AND asset_id=? AND added_by=?", id, assetID, access.Current(r).ID).Scan(&n); e == sql.ErrNoRows {
			http.Error(w, "contributors can only remove their own additions", 403)
			return
		} else if e != nil {
			s.fail(w, e)
			return
		}
		q += " AND added_by=" + strconv.FormatInt(access.Current(r).ID, 10)
	}
	_, e = s.DB.ExecContext(r.Context(), q, id, assetID)
	if e != nil {
		s.fail(w, e)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) editAlbum(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r, "id")
	if e != nil {
		http.Error(w, "invalid ID", 400)
		return
	}
	if s.albumRole(r, id) != "owner" {
		http.Error(w, "album owner required", 403)
		return
	}
	if r.Method == "DELETE" {
		_, e = s.DB.ExecContext(r.Context(), "DELETE FROM albums WHERE id=?", id)
	} else {
		var b album
		if json.NewDecoder(r.Body).Decode(&b) != nil || len([]rune(strings.TrimSpace(b.Name))) < 1 || len([]rune(b.Name)) > 120 || len(b.Description) > 2000 {
			http.Error(w, "invalid album details", 400)
			return
		}
		_, e = s.DB.ExecContext(r.Context(), "UPDATE albums SET name=?,description=? WHERE id=?", strings.TrimSpace(b.Name), b.Description, id)
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) members(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r, "id")
	if e != nil {
		http.Error(w, "invalid ID", 400)
		return
	}
	if s.albumRole(r, id) != "owner" {
		http.Error(w, "album owner required", 403)
		return
	}
	if r.Method == "GET" {
		rows, e := s.DB.QueryContext(r.Context(), "SELECT u.id,u.username,u.display_name,m.role,m.starts_at,m.expires_at FROM album_members m JOIN users u ON u.id=m.user_id WHERE m.album_id=? ORDER BY u.username", id)
		if e != nil {
			s.fail(w, e)
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var uid, starts, expires int64
			var username, name, role string
			if e = rows.Scan(&uid, &username, &name, &role, &starts, &expires); e != nil {
				s.fail(w, e)
				return
			}
			items = append(items, map[string]any{"id": uid, "username": username, "displayName": name, "role": role, "startsAt": starts, "expiresAt": expires})
		}
		if e = rows.Err(); e != nil {
			s.fail(w, e)
			return
		}
		send(w, map[string]any{"items": items})
		return
	}
	if r.Method == "DELETE" {
		uid, e := idParam(r, "userId")
		if e != nil {
			http.Error(w, "invalid user", 400)
			return
		}
		_, e = s.DB.ExecContext(r.Context(), "DELETE FROM album_members WHERE album_id=? AND user_id=?", id, uid)
		if e != nil {
			s.fail(w, e)
			return
		}
	} else {
		var b struct {
			Username  string `json:"username"`
			Role      string `json:"role"`
			StartsAt  string `json:"startsAt"`
			ExpiresAt string `json:"expiresAt"`
		}
		if json.NewDecoder(r.Body).Decode(&b) != nil || (b.Role != "viewer" && b.Role != "contributor") {
			http.Error(w, "username and viewer/contributor role required", 400)
			return
		}
		starts, expires, e := shareWindow(b.StartsAt, b.ExpiresAt)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		var uid int64
		e = s.DB.QueryRowContext(r.Context(), "SELECT id FROM users WHERE username=? AND enabled=1 AND id<>(SELECT owner_id FROM albums WHERE id=?)", strings.TrimSpace(b.Username), id).Scan(&uid)
		if e == sql.ErrNoRows {
			http.Error(w, "recipient unavailable", 400)
			return
		}
		if e != nil {
			s.fail(w, e)
			return
		}
		_, e = s.DB.ExecContext(r.Context(), "INSERT INTO album_members(album_id,user_id,role,starts_at,expires_at) VALUES(?,?,?,?,?) ON CONFLICT(album_id,user_id) DO UPDATE SET role=excluded.role,starts_at=excluded.starts_at,expires_at=excluded.expires_at", id, uid, b.Role, starts, expires)
		if e != nil {
			s.fail(w, e)
			return
		}
	}
	w.WriteHeader(204)
}
