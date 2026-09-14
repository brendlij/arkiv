package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"gallery/internal/access"
	"gallery/internal/library"
	"gallery/internal/scanner"
	"gallery/internal/thumbnails"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type Server struct {
	cacheMu         sync.Mutex
	cacheRun        sync.Mutex
	cacheState      CacheStatus
	UploadQuota     int64
	UploadRetention time.Duration
	DB              *sql.DB
	Cache           string
	Uploads         string
	Scanner         *scanner.Scanner
	RequestScan     func() bool
}

func (s *Server) Register(m *http.ServeMux) {
	s.registerWorkflows(m)
	m.HandleFunc("GET /api/cache", s.cacheStatus)
	m.HandleFunc("POST /api/cache", s.cacheStatus)
	m.HandleFunc("GET /api/assets/{id}/playback", s.playback)
	m.HandleFunc("POST /api/assets/{id}/playback", s.playback)
	m.HandleFunc("GET /api/assets/{id}/playback/file", s.playbackFile)
	m.HandleFunc("GET /api/processing", s.processing)
	m.HandleFunc("POST /api/processing/retry", s.retryProcessing)
	m.HandleFunc("POST /api/uploads/sessions", s.startUpload)
	m.HandleFunc("GET /api/uploads/sessions", s.listUploads)
	m.HandleFunc("GET /api/uploads/sessions/{uploadID}", s.uploadSession)
	m.HandleFunc("PATCH /api/uploads/sessions/{uploadID}", s.uploadChunk)
	m.HandleFunc("POST /api/uploads/sessions/{uploadID}/complete", s.completeUpload)
	m.HandleFunc("DELETE /api/uploads/sessions/{uploadID}", s.cancelUpload)
	m.HandleFunc("GET /api/admin/users/{userId}/storage", s.userStorage)
	m.HandleFunc("POST /api/admin/users/{userId}/storage", s.userStorage)

	m.HandleFunc("POST /api/assets/bulk", s.bulkAssets)
	m.HandleFunc("GET /api/me/upload-folders", s.uploadFolders)
	m.HandleFunc("GET /api/me/folders/sharing", s.folderSharing)
	m.HandleFunc("POST /api/me/folders/manage", s.manageFolder)
	m.HandleFunc("GET /api/access-details", s.accessDetails)
	m.HandleFunc("GET /api/assets", s.assets)
	m.HandleFunc("GET /api/timeline", s.assets)
	m.HandleFunc("GET /api/search", s.assets)
	m.HandleFunc("GET /api/assets/{id}", s.asset)
	m.HandleFunc("GET /api/assets/{id}/thumbnail/{size}", s.file)
	m.HandleFunc("GET /api/assets/{id}/original", s.file)
	m.HandleFunc("GET /api/assets/{id}/full-image", s.fullImage)
	m.HandleFunc("POST /api/assets/{id}/favorite", s.favorite)
	m.HandleFunc("GET /api/folders", s.folders)
	m.HandleFunc("GET /api/stats", s.stats)
	m.HandleFunc("GET /api/albums", s.albums)
	m.HandleFunc("POST /api/albums", s.createAlbum)
	m.HandleFunc("GET /api/albums/{id}", s.album)
	m.HandleFunc("POST /api/albums/{id}/assets", s.addAlbumAsset)
	m.HandleFunc("DELETE /api/albums/{id}/assets/{assetId}", s.removeAlbumAsset)
	m.HandleFunc("POST /api/scan", func(w http.ResponseWriter, r *http.Request) {
		if !access.Admin(w, r) {
			return
		}
		if s.RequestScan == nil || !s.RequestScan() {
			http.Error(w, "scan already queued or running", 409)
			return
		}
		w.WriteHeader(202)
	})
	m.HandleFunc("POST /api/retry", s.retry)
}
func send(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if e := json.NewEncoder(w).Encode(v); e != nil {
		slog.Warn("response write failed", "error", e)
	}
}
func (s *Server) fail(w http.ResponseWriter, e error) {
	slog.Error("API request failed", "error", e)
	http.Error(w, "internal server error", 500)
}
func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r, "id")
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	var root, rel, mimeType, key, status string
	e = s.DB.QueryRowContext(r.Context(), "SELECT l.root,a.relative_path,a.mime_type,a.preview_key,a.preview_status FROM assets a JOIN libraries l ON l.id=a.library_id WHERE a.id=? AND l.enabled=1 AND "+access.ReadFile(access.Current(r), "a"), id).Scan(&root, &rel, &mimeType, &key, &status)
	if e == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	var path string
	if v := r.PathValue("size"); v != "" {
		size, err := strconv.Atoi(v)
		if err != nil || (size != 256 && size != 1024 && size != 2048) {
			http.Error(w, "unsupported size", 400)
			return
		}
		if len(key) != 64 || status != "ready" {
			http.NotFound(w, r)
			return
		}
		path = thumbnails.Path(s.Cache, key, size)
		mimeType = "image/webp"
	} else {
		path, e = library.Resolve(root, rel)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		disposition := "inline"
		if r.URL.Query().Get("download") == "1" || mimeType == "application/octet-stream" {
			disposition = "attachment"
		}
		w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filepath.Base(rel)}))
	}
	var f *os.File
	if r.PathValue("size") == "" {
		f, e = library.Open(root, rel)
	} else {
		f, e = os.Open(path)
	}
	if e != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	if r.PathValue("size") != "" {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("ETag", fmt.Sprintf(`"%s-%s"`, key, r.PathValue("size")))
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}
func (s *Server) favorite(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r, "id")
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	var body struct {
		Favorite *bool `json:"favorite"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Favorite == nil {
		http.Error(w, "favorite boolean required", 400)
		return
	}
	var exists int
	if e = s.DB.QueryRowContext(r.Context(), "SELECT 1 FROM assets a JOIN libraries l ON l.id=a.library_id WHERE a.id=? AND l.enabled=1 AND "+access.Visible(access.Current(r), "a"), id).Scan(&exists); e == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if e != nil {
		s.fail(w, e)
		return
	}
	if *body.Favorite {
		_, e = s.DB.ExecContext(r.Context(), "INSERT INTO user_favorites(user_id,asset_id) VALUES(?,?) ON CONFLICT DO NOTHING", access.Current(r).ID, id)
	} else {
		_, e = s.DB.ExecContext(r.Context(), "DELETE FROM user_favorites WHERE user_id=? AND asset_id=?", access.Current(r).ID, id)
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	send(w, map[string]any{"favorite": *body.Favorite})
}
func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	var assets, images, videos, favorites, queue, failed int64
	e := s.DB.QueryRowContext(r.Context(), `SELECT count(*),coalesce(sum(a.media_type<>'video'),0),coalesce(sum(a.media_type='video'),0),coalesce(sum(`+access.Favorite(access.Current(r))+`),0),coalesce(sum(a.preview_status='failed' OR a.metadata_status='failed'),0) FROM assets a JOIN libraries l ON l.id=a.library_id WHERE l.enabled=1 AND `+access.Visible(access.Current(r), "a")).Scan(&assets, &images, &videos, &favorites, &failed)
	if e != nil {
		s.fail(w, e)
		return
	}
	e = s.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM jobs j JOIN assets a ON a.id=j.asset_id JOIN libraries l ON l.id=a.library_id WHERE l.enabled=1 AND j.state<>'failed' AND "+access.Visible(access.Current(r), "a")).Scan(&queue)
	if e != nil {
		s.fail(w, e)
		return
	}
	running := s.Scanner != nil && s.Scanner.Running.Load()
	lastScan, scanError := "", ""
	if s.Scanner != nil {
		lastScan, scanError = s.Scanner.Status()
	}
	if access.Current(r).Role != "admin" {
		scanError = ""
	}
	send(w, map[string]any{"assets": assets, "images": images, "videos": videos, "favorites": favorites, "thumbnailQueue": queue, "failed": failed, "scanRunning": running, "lastScan": lastScan, "scanError": scanError})
}
func (s *Server) retry(w http.ResponseWriter, r *http.Request) {
	if !access.Admin(w, r) {
		return
	}
	_, e := s.DB.ExecContext(r.Context(), "UPDATE jobs SET state='pending',attempts=0,next_at=0 WHERE state='failed'")
	if e != nil {
		s.fail(w, e)
		return
	} // Also restore missing cache derivatives after cache replacement.
	_, e = s.DB.ExecContext(r.Context(), "INSERT INTO jobs(asset_id) SELECT id FROM assets WHERE preview_status='ready' AND NOT EXISTS(SELECT 1 FROM media_deletions md WHERE md.asset_id=assets.id) ON CONFLICT(asset_id) DO NOTHING")
	if e != nil {
		s.fail(w, e)
		return
	}
	w.WriteHeader(202)
}
