package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"gallery/internal/access"
	"gallery/internal/library"
	"gallery/internal/thumbnails"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type playbackAsset struct {
	id, generation, size, mtime int64
	root, rel                   string
}

func (a playbackAsset) key() string { return thumbnails.Key(a.id, a.generation, a.size, a.mtime) }
func (s *Server) playbackAsset(r *http.Request) (a playbackAsset, e error) {
	a.id, e = idParam(r, "id")
	if e != nil {
		return
	}
	e = s.DB.QueryRowContext(r.Context(), "SELECT a.generation,a.file_size,a.modified_at,l.root,a.relative_path FROM assets a JOIN libraries l ON l.id=a.library_id WHERE a.id=? AND l.enabled=1 AND a.media_type='video' AND "+access.ReadFile(access.Current(r), "a"), a.id).Scan(&a.generation, &a.size, &a.mtime, &a.root, &a.rel)
	return
}
func (s *Server) playback(w http.ResponseWriter, r *http.Request) {
	a, e := s.playbackAsset(r)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	state := "missing"
	var generation int64
	e = s.DB.QueryRowContext(r.Context(), "SELECT generation,state FROM video_jobs WHERE asset_id=?", a.id).Scan(&generation, &state)
	if e != nil && e != sql.ErrNoRows {
		s.fail(w, e)
		return
	}
	if generation != a.generation {
		state = "missing"
	}
	if state == "ready" {
		if i, e := os.Stat(filepath.Join(s.Cache, "video-v1", a.key()+".mp4")); e != nil || i.Size() == 0 {
			state = "missing"
		}
	}
	if r.Method == "POST" && (state == "missing" || state == "failed") {
		res, e := s.DB.ExecContext(r.Context(), `INSERT INTO video_jobs(asset_id,generation,state) SELECT ?,?,'pending' WHERE (SELECT count(*) FROM video_jobs WHERE state IN ('pending','running'))<32 ON CONFLICT(asset_id) DO UPDATE SET generation=excluded.generation,state='pending',error='' WHERE video_jobs.state NOT IN ('pending','running') OR video_jobs.generation!=excluded.generation`, a.id, a.generation)
		if e != nil {
			s.fail(w, e)
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			http.Error(w, "Video queue is busy. Try again shortly.", 429)
			return
		}
		state = "pending"
	}
	message := ""
	if state == "failed" {
		message = "Conversion failed. HDR or unsupported files may require an external player. You can retry or download the original."
	}
	send(w, map[string]string{"state": state, "error": message})
}
func (s *Server) playbackFile(w http.ResponseWriter, r *http.Request) {
	a, e := s.playbackAsset(r)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	var ready bool
	e = s.DB.QueryRowContext(r.Context(), "SELECT state='ready' AND generation=? FROM video_jobs WHERE asset_id=?", a.generation, a.id).Scan(&ready)
	if e != nil || !ready {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.Cache, "video-v1", a.key()+".mp4")
	f, e := os.Open(path)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		s.fail(w, e)
		return
	}
	_ = os.Chtimes(path, time.Now(), time.Now())
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeContent(w, r, "playback.mp4", st.ModTime(), f)
}
func (s *Server) RunVideoJobs(ctx context.Context) {
	_, _ = s.DB.ExecContext(ctx, "UPDATE video_jobs SET state='pending' WHERE state='running'")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		var a playbackAsset
		e := s.DB.QueryRowContext(ctx, `SELECT a.id,a.generation,a.file_size,a.modified_at,l.root,a.relative_path FROM video_jobs j JOIN assets a ON a.id=j.asset_id JOIN libraries l ON l.id=a.library_id WHERE j.state='pending' AND l.enabled=1 ORDER BY a.id LIMIT 1`).Scan(&a.id, &a.generation, &a.size, &a.mtime, &a.root, &a.rel)
		if e == nil {
			_, e = s.DB.ExecContext(ctx, "UPDATE video_jobs SET state='running',generation=? WHERE asset_id=?", a.generation, a.id)
			if e == nil {
				e = s.convertVideo(ctx, a)
			}
			state, message := "ready", ""
			if e != nil {
				state = "failed"
				message = e.Error()
			}
			if ctx.Err() != nil {
				return
			}
			_, _ = s.DB.ExecContext(ctx, "UPDATE video_jobs SET state=?,error=? WHERE asset_id=? AND generation=?", state, message, a.id, a.generation)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) convertVideo(parent context.Context, a playbackAsset) error {
	ctx, cancel := context.WithTimeout(parent, 6*time.Hour)
	defer cancel()
	source, e := library.Resolve(a.root, a.rel)
	if e != nil {
		return e
	}
	st, e := os.Stat(source)
	if e != nil {
		return e
	}
	if st.Size() != a.size || st.ModTime().UnixNano() != a.mtime {
		return fmt.Errorf("source changed; rescan required")
	}
	// Avoid silently rendering HDR with washed-out colours. Original remains available.
	probe := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=color_transfer", "-of", "json", source)
	out, e := probe.Output()
	if e != nil {
		return fmt.Errorf("video probe failed: %w", e)
	}
	var p struct {
		Streams []struct {
			Transfer string `json:"color_transfer"`
		} `json:"streams"`
	}
	if e = json.Unmarshal(out, &p); e != nil {
		return e
	}
	for _, v := range p.Streams {
		if v.Transfer == "smpte2084" || v.Transfer == "arib-std-b67" {
			return fmt.Errorf("HDR conversion is not supported; use original")
		}
	}
	dir := filepath.Join(s.Cache, "video-v1")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	temp, e := os.MkdirTemp(dir, "transcode-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	target := filepath.Join(temp, "playback.mp4")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-i", source, "-map", "0:v:0", "-map", "0:a:0?", "-sn", "-dn", "-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2", "-c:v", "libx264", "-preset", "fast", "-crf", "18", "-pix_fmt", "yuv420p", "-threads", "2", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", "-y", target)
	var stderr limitedVideoLog
	cmd.Stderr = &stderr
	if e = cmd.Run(); e != nil {
		return fmt.Errorf("conversion failed: %w: %s", e, stderr.String())
	}
	st, e = os.Stat(source)
	if e != nil {
		return e
	}
	if st.Size() != a.size || st.ModTime().UnixNano() != a.mtime {
		return fmt.Errorf("source changed during conversion")
	}
	var generation int64
	e = s.DB.QueryRowContext(ctx, "SELECT generation FROM assets WHERE id=? AND NOT EXISTS(SELECT 1 FROM media_deletions WHERE asset_id=assets.id)", a.id).Scan(&generation)
	if e != nil {
		return e
	}
	if generation != a.generation {
		return fmt.Errorf("source generation changed")
	}
	return os.Rename(target, filepath.Join(dir, a.key()+".mp4"))
}

type limitedVideoLog struct{ bytes.Buffer }

func (b *limitedVideoLog) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 8192 {
		left := 8192 - b.Len()
		if len(p) > left {
			p = p[:left]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}
