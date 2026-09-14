package api

import (
	"context"
	"fmt"
	"gallery/internal/access"
	"gallery/internal/thumbnails"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CacheStatus struct {
	CheckedAt    string `json:"checkedAt"`
	Bytes        int64  `json:"bytes"`
	Reclaimable  int64  `json:"reclaimable"`
	RemovedBytes int64  `json:"removedBytes"`
	Missing      int    `json:"missing"`
	Requeued     int    `json:"requeued"`
	Error        string `json:"error"`
}

func (s *Server) cacheStatus(w http.ResponseWriter, r *http.Request) {
	if !access.Admin(w, r) {
		return
	}
	if r.Method == "POST" {
		if !s.cacheRun.TryLock() {
			http.Error(w, "Cache check already running", 409)
			return
		}
		defer s.cacheRun.Unlock()
		if e := s.auditCache(r.Context(), true); e != nil {
			s.fail(w, e)
			return
		}
	}
	s.cacheMu.Lock()
	status := s.cacheState
	s.cacheMu.Unlock()
	var originals, trash int64
	e := s.DB.QueryRowContext(r.Context(), "SELECT coalesce(sum(file_size),0),coalesce(sum(CASE WHEN trashed_at>0 THEN file_size ELSE 0 END),0) FROM assets WHERE library_id='arkiv-uploads'").Scan(&originals, &trash)
	if e != nil {
		s.fail(w, e)
		return
	}
	send(w, map[string]any{"cache": status, "uploadedBytes": originals, "trashBytes": trash})
}
func (s *Server) MaintainCache(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if s.cacheRun.TryLock() {
			if e := s.auditCache(ctx, true); e != nil && ctx.Err() == nil {
				slog.Warn("cache maintenance incomplete", "error", e)
			}
			s.cacheRun.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func hexKey(key string) bool { return len(key) == 64 && strings.Trim(key, "0123456789abcdef") == "" }

// Recognize only our generated files; unrelated files and symlinks stay untouched.
func generatedCache(path string) (key string, expendable bool, ok bool) {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) == 2 && (parts[0] == "full-images-v1" || parts[0] == "video-v1") {
		ext := ".png"
		if parts[0] == "video-v1" {
			ext = ".mp4"
		}
		key = strings.TrimSuffix(parts[1], ext)
		return key, true, strings.HasSuffix(parts[1], ext) && hexKey(key)
	}
	if len(parts) == 3 && len(parts[0]) == 2 && len(parts[1]) == 2 {
		for _, size := range thumbnails.Sizes {
			suffix := fmt.Sprintf("-%d.webp", size)
			key = strings.TrimSuffix(parts[2], suffix)
			if strings.HasSuffix(parts[2], suffix) && hexKey(key) && parts[0] == key[:2] && parts[1] == key[2:4] {
				return key, false, true
			}
		}
		if strings.HasPrefix(parts[2], ".preview-") && strings.HasSuffix(parts[2], ".webp") {
			return "", false, true
		}
	}
	// Interrupted converter/worker workspaces never contain authoritative originals.
	if len(parts) >= 2 && strings.HasPrefix(parts[0], "processing-") {
		return "", false, true
	}
	if len(parts) >= 3 && ((parts[0] == "full-images-v1" && strings.HasPrefix(parts[1], "render-")) || (parts[0] == "video-v1" && strings.HasPrefix(parts[1], "transcode-"))) {
		return "", false, true
	}
	return "", false, false
}
func (s *Server) auditCache(ctx context.Context, clean bool) (err error) {
	result := CacheStatus{CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	defer func() {
		if err != nil {
			result.Error = err.Error()
		}
		s.cacheMu.Lock()
		s.cacheState = result
		s.cacheMu.Unlock()
	}()
	root, e := os.OpenRoot(s.Cache)
	if e != nil {
		return e
	}
	defer root.Close()
	type asset struct {
		id, generation int64
		key            string
		ready, enabled bool
	}
	assets := []asset{}
	keep := map[string]bool{}
	rows, e := s.DB.QueryContext(ctx, "SELECT a.id,a.generation,a.file_size,a.modified_at,a.preview_key,a.preview_status='ready',l.enabled FROM assets a JOIN libraries l ON l.id=a.library_id")
	if e != nil {
		return e
	}
	for rows.Next() {
		var a asset
		var size, mtime int64
		if e = rows.Scan(&a.id, &a.generation, &size, &mtime, &a.key, &a.ready, &a.enabled); e != nil {
			break
		}
		keep[thumbnails.Key(a.id, a.generation, size, mtime)] = true
		if hexKey(a.key) {
			keep[a.key] = true
		}
		assets = append(assets, a)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return e
	}
	dirs := []string{}
	err = fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			if path != "." {
				dirs = append(dirs, path)
			}
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		result.Bytes += info.Size()
		key, expendable, known := generatedCache(path)
		age := time.Since(info.ModTime())
		stale := known && ((!keep[key] && age > 24*time.Hour) || (expendable && age > 30*24*time.Hour))
		if !stale {
			return nil
		}
		result.Reclaimable += info.Size()
		if clean {
			if e = root.Remove(path); e != nil {
				return e
			}
			result.RemovedBytes += info.Size()
			result.Bytes -= info.Size()
		}
		return nil
	})
	if err != nil {
		return err
	}
	if clean {
		for i := len(dirs) - 1; i >= 0; i-- { // Empty directories only; never recurse through deletion.
			path := dirs[i]
			if _, _, known := generatedCache(path + "/.preview-test.webp"); known || strings.HasPrefix(path, "full-images-v1/render-") || strings.HasPrefix(path, "video-v1/transcode-") || strings.HasPrefix(path, "processing-") {
				_ = root.Remove(path)
			}
		}
	}
	for _, a := range assets {
		if !a.ready || !a.enabled || !hexKey(a.key) {
			continue
		}
		missing := false
		for _, size := range thumbnails.Sizes {
			info, e := root.Stat(thumbnails.Path("", a.key, size))
			if os.IsNotExist(e) || (e == nil && info.Size() == 0) {
				missing = true
			} else if e != nil {
				return e
			}
		}
		if !missing {
			continue
		}
		result.Missing++
		if !clean {
			continue
		}
		tx, e := s.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		r, e := tx.ExecContext(ctx, "INSERT INTO jobs(asset_id) SELECT id FROM assets WHERE id=? AND generation=? AND NOT EXISTS(SELECT 1 FROM media_deletions md WHERE md.asset_id=assets.id) ON CONFLICT(asset_id) DO NOTHING", a.id, a.generation)
		var n int64
		if e == nil {
			n, e = r.RowsAffected()
		}
		if e == nil && n > 0 {
			_, e = tx.ExecContext(ctx, "UPDATE assets SET preview_status='pending' WHERE id=? AND generation=?", a.id, a.generation)
		}
		if e != nil {
			tx.Rollback()
			return e
		}
		if e = tx.Commit(); e != nil {
			return e
		}
		result.Requeued += int(n)
	}
	return nil
}
