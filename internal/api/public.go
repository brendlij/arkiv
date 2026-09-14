package api

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gallery/internal/access"
	"net/http"
	"strconv"
	"time"
)

// Zero means immediate / no expiration. All persisted values are UTC Unix seconds.
func shareWindow(start, end string) (int64, int64, error) {
	var starts, expires int64
	for i, v := range []string{start, end} {
		if v == "" {
			continue
		}
		t, e := time.Parse(time.RFC3339, v)
		if e != nil || t.Unix() <= 0 {
			return 0, 0, fmt.Errorf("use a valid date and time")
		}
		if i == 0 {
			starts = t.Unix()
		} else {
			expires = t.Unix()
		}
	}
	if expires != 0 && (expires <= time.Now().Unix() || expires <= starts) {
		return 0, 0, fmt.Errorf("expiration must be in the future and after the start")
	}
	return starts, expires, nil
}
func shareHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

type publicShare struct {
	AlbumID        int64 `json:"-"`
	StartsAt       int64 `json:"startsAt"`
	ExpiresAt      int64 `json:"expiresAt"`
	AllowDownloads bool  `json:"allowDownloads"`
	CreatedAt      int64 `json:"createdAt"`
}

func (s *Server) publicSettings(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r, "id")
	if e != nil {
		http.Error(w, "invalid album", 400)
		return
	}
	if s.albumRole(r, id) != "owner" {
		http.Error(w, "album owner required", 403)
		return
	}
	if r.Method == "GET" {
		var p publicShare
		e = s.DB.QueryRowContext(r.Context(), "SELECT starts_at,expires_at,allow_downloads,created_at FROM album_public_shares WHERE album_id=?", id).Scan(&p.StartsAt, &p.ExpiresAt, &p.AllowDownloads, &p.CreatedAt)
		if e == sql.ErrNoRows {
			send(w, map[string]any{"share": nil})
			return
		}
		if e != nil {
			s.fail(w, e)
			return
		}
		send(w, map[string]any{"share": p})
		return
	}
	if r.Method == "DELETE" {
		if _, e = s.DB.ExecContext(r.Context(), "DELETE FROM album_public_shares WHERE album_id=?", id); e != nil {
			s.fail(w, e)
			return
		}
		w.WriteHeader(204)
		return
	}
	var b struct {
		StartsAt       string `json:"startsAt"`
		ExpiresAt      string `json:"expiresAt"`
		AllowDownloads bool   `json:"allowDownloads"`
	}
	if json.NewDecoder(r.Body).Decode(&b) != nil {
		http.Error(w, "invalid settings", 400)
		return
	}
	starts, expires, e := shareWindow(b.StartsAt, b.ExpiresAt)
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	secret := make([]byte, 32)
	if _, e = rand.Read(secret); e != nil {
		s.fail(w, e)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	_, e = s.DB.ExecContext(r.Context(), "INSERT INTO album_public_shares(album_id,token_hash,starts_at,expires_at,allow_downloads,created_at) VALUES(?,?,?,?,?,?) ON CONFLICT(album_id) DO UPDATE SET token_hash=excluded.token_hash,starts_at=excluded.starts_at,expires_at=excluded.expires_at,allow_downloads=excluded.allow_downloads,created_at=excluded.created_at", id, shareHash(token), starts, expires, b.AllowDownloads, time.Now().Unix())
	if e != nil {
		s.fail(w, e)
		return
	}
	send(w, map[string]any{"path": "/share/" + token, "share": publicShare{StartsAt: starts, ExpiresAt: expires, AllowDownloads: b.AllowDownloads, CreatedAt: time.Now().Unix()}})
}
func (s *Server) resolvePublic(w http.ResponseWriter, r *http.Request) (publicShare, bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	var p publicShare
	token := r.PathValue("token")
	raw, e := base64.RawURLEncoding.DecodeString(token)
	if e != nil || len(raw) != 32 {
		http.NotFound(w, r)
		return p, false
	}
	e = s.DB.QueryRowContext(r.Context(), `SELECT p.album_id,p.starts_at,p.expires_at,p.allow_downloads,p.created_at FROM album_public_shares p JOIN albums al ON al.id=p.album_id JOIN users u ON u.id=al.owner_id AND u.enabled=1 WHERE p.token_hash=? AND p.starts_at<=unixepoch() AND (p.expires_at=0 OR p.expires_at>unixepoch())`, shareHash(token)).Scan(&p.AlbumID, &p.StartsAt, &p.ExpiresAt, &p.AllowDownloads, &p.CreatedAt)
	if e == sql.ErrNoRows {
		http.NotFound(w, r)
		return p, false
	}
	if e != nil {
		s.fail(w, e)
		return p, false
	}
	return p, true
}
func (s *Server) publicFile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.resolvePublic(w, r)
	if !ok {
		return
	}
	if r.PathValue("size") == "" && !p.AllowDownloads {
		http.Error(w, "original downloads are disabled", 403)
		return
	}
	s.file(w, r.WithContext(access.WithUser(r.Context(), access.User{PublicAlbum: p.AlbumID})))
}
func (s *Server) publicAlbum(w http.ResponseWriter, r *http.Request) {
	p, ok := s.resolvePublic(w, r)
	if !ok {
		return
	}
	var name, description string
	if e := s.DB.QueryRowContext(r.Context(), "SELECT name,description FROM albums WHERE id=?", p.AlbumID).Scan(&name, &description); e != nil {
		s.fail(w, e)
		return
	}
	cursor := int64(0)
	if v := r.URL.Query().Get("cursor"); v != "" {
		var e error
		cursor, e = strconv.ParseInt(v, 10, 64)
		if e != nil || cursor < 1 {
			http.Error(w, "invalid cursor", 400)
			return
		}
	}
	type item struct {
		ID            int64  `json:"id"`
		Filename      string `json:"filename"`
		MediaType     string `json:"mediaType"`
		PreviewStatus string `json:"previewStatus"`
	}
	items := []item{}
	rows, e := s.DB.QueryContext(r.Context(), "SELECT a.id,a.filename,a.media_type,a.preview_status FROM assets a JOIN libraries l ON l.id=a.library_id AND l.enabled=1 JOIN album_assets pa ON pa.asset_id=a.id AND pa.album_id=? WHERE a.id>? AND "+access.Visible(access.User{PublicAlbum: p.AlbumID}, "a")+" ORDER BY a.id LIMIT 121", p.AlbumID, cursor)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var a item
		if e = rows.Scan(&a.ID, &a.Filename, &a.MediaType, &a.PreviewStatus); e != nil {
			s.fail(w, e)
			return
		}
		items = append(items, a)
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	next := ""
	if len(items) > 120 {
		items = items[:120]
		next = strconv.FormatInt(items[len(items)-1].ID, 10)
	}
	send(w, map[string]any{"name": name, "description": description, "items": items, "nextCursor": next, "allowDownloads": p.AllowDownloads, "expiresAt": p.ExpiresAt})
}
