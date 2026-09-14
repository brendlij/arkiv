package api

import (
	"context"
	"fmt"
	"gallery/internal/media"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *Server) MaintainUploads(ctx context.Context) {
	if s.Uploads == "" {
		return
	}
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if e := s.cleanUploads(ctx); e != nil && ctx.Err() == nil {
			slog.Warn("upload maintenance incomplete", "error", e)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) cleanUploads(ctx context.Context) error {
	if !uploadLock.TryLock() {
		return nil
	}
	defer uploadLock.Unlock()
	if e := s.finishMediaDeletions(ctx); e != nil {
		slog.Warn("media deletion pending", "error", e)
	}
	root, e := os.OpenRoot(s.Uploads)
	if e != nil {
		return e
	}
	defer root.Close()
	cutoff := time.Now().Add(-s.retention()).Unix()
	rows, e := s.DB.QueryContext(ctx, "SELECT "+uploadColumns+" FROM upload_sessions")
	if e != nil {
		return e
	}
	sessions := []uploadRecord{}
	for rows.Next() {
		v, err := readUpload(rows)
		if err != nil {
			rows.Close()
			return err
		}
		sessions = append(sessions, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	protected := map[string]bool{}
	for _, v := range sessions {
		temp := ".incoming/" + v.ID + ".part"
		dest := fmt.Sprintf("user-%d/%s%s", v.Owner, v.ID, strings.ToLower(filepath.Ext(v.Filename)))
		if v.State == "committing" {
			if _, err := s.finishUpload(ctx, v); err != nil {
				protected[temp] = true
				protected[dest] = true
				slog.Warn("upload finalization will retry", "upload", v.ID, "error", err)
			}
			continue
		}
		if v.Updated > cutoff {
			protected[temp] = true
			continue
		}
		if v.State == "uploading" {
			if e = root.Remove(temp); e != nil && !os.IsNotExist(e) {
				return e
			}
		}
		if _, e = s.DB.ExecContext(ctx, "DELETE FROM upload_sessions WHERE id=?", v.ID); e != nil {
			return e
		}
	}
	entries, e := readRootDir(root, ".incoming")
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		path := ".incoming/" + entry.Name()
		if !entry.Type().IsRegular() || protected[path] || !strings.HasSuffix(entry.Name(), ".part") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().Unix() < cutoff {
			if e = root.Remove(path); e != nil {
				return e
			}
		}
	}
	// Never delete an unindexed complete original: quarantine generated upload
	// files after the grace period, preserving recovery after crashes/DB restores.
	dirs, e := readRootDir(root, ".")
	if e != nil {
		return e
	}
	for _, dir := range dirs {
		if !dir.IsDir() || dir.Type()&os.ModeSymlink != 0 || !strings.HasPrefix(dir.Name(), "user-") {
			continue
		}
		uid, err := strconv.ParseInt(strings.TrimPrefix(dir.Name(), "user-"), 10, 64)
		if err != nil || uid < 1 {
			continue
		}
		var user int
		if e = s.DB.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE id=?", uid).Scan(&user); e != nil {
			return e
		}
		if user == 0 {
			continue
		}
		files, err := readRootDir(root, dir.Name())
		if err != nil {
			return err
		}
		for _, entry := range files {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !entry.Type().IsRegular() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			stem := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			if len(stem) < 32 || len(stem) > 80 || strings.Trim(stem, "0123456789abcdef") != "" {
				continue
			}
			path := dir.Name() + "/" + entry.Name()
			if protected[path] {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.ModTime().Unix() >= cutoff {
				continue
			}
			var indexed int
			if e = s.DB.QueryRowContext(ctx, "SELECT count(*) FROM assets WHERE library_id='arkiv-uploads' AND relative_path=?", path).Scan(&indexed); e != nil {
				return e
			}
			if indexed > 0 {
				continue
			}
			if ext == ".partial" {
				if e = root.Remove(path); e != nil {
					return e
				}
				continue
			}
			if _, ok := media.Lookup(ext); !ok {
				continue
			}
			quarantine := ".quarantine/" + path
			if e = root.MkdirAll(".quarantine/"+dir.Name(), 0700); e != nil {
				return e
			}
			if _, err = root.Stat(quarantine); err == nil {
				continue
			} else if !os.IsNotExist(err) {
				return err
			}
			if _, e = s.DB.ExecContext(ctx, "INSERT OR IGNORE INTO upload_quarantine(path,original_path,owner_id,file_size,created_at) VALUES(?,?,?,?,?)", quarantine, path, uid, info.Size(), time.Now().Unix()); e != nil {
				return e
			}
			if e = root.Rename(path, quarantine); e != nil {
				return e
			}
			slog.Warn("unindexed upload preserved in quarantine", "path", quarantine)
		}
	}
	return nil
}
func readRootDir(root *os.Root, path string) ([]os.DirEntry, error) {
	f, e := root.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return f.ReadDir(-1)
}
