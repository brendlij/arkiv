package api

import (
	"context"
	"errors"
	"fmt"
	"gallery/internal/thumbnails"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Called with uploadLock held. Intent is durable before any original is removed;
// interrupted deletions remain hidden and retry at startup/hourly maintenance.
func (s *Server) finishMediaDeletions(ctx context.Context) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT a.id,a.relative_path,a.preview_key FROM media_deletions d JOIN assets a ON a.id=d.asset_id WHERE a.library_id='arkiv-uploads'`)
	if err != nil {
		return err
	}
	type deletion struct {
		id        int64
		path, key string
	}
	items := []deletion{}
	for rows.Next() {
		var v deletion
		if err = rows.Scan(&v.id, &v.path, &v.key); err != nil {
			break
		}
		items = append(items, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	root, err := os.OpenRoot(s.Uploads)
	if err != nil {
		return err
	}
	defer root.Close()
	var failures []error
	for _, v := range items {
		deletionErr := func() error {
			parts := strings.Split(v.path, "/")
			if len(parts) != 2 || !strings.HasPrefix(parts[0], "user-") || parts[1] == "" || strings.ContainsAny(v.path, "\\:") || parts[1] == ".." || parts[1] == "." {
				return fmt.Errorf("invalid managed original path")
			}
			uid, e := strconv.ParseInt(strings.TrimPrefix(parts[0], "user-"), 10, 64)
			if e != nil || uid < 1 {
				return fmt.Errorf("invalid upload owner path")
			}
			dir, e := root.Lstat(parts[0])
			if e == nil && (!dir.IsDir() || dir.Mode()&os.ModeSymlink != 0) {
				return fmt.Errorf("upload folder must be a real directory")
			}
			if e != nil && !os.IsNotExist(e) {
				return e
			}
			info, e := root.Lstat(v.path)
			if e == nil {
				if !info.Mode().IsRegular() {
					return fmt.Errorf("managed original is not a regular file")
				}
				e = root.Remove(v.path)
			}
			if e != nil && !os.IsNotExist(e) {
				return e
			}
			// Cache is disposable, but only remove this asset's known derivative files.
			if len(v.key) == 64 && strings.Trim(v.key, "0123456789abcdef") == "" && s.Cache != "" {
				cache, e := os.OpenRoot(s.Cache)
				if e != nil && !os.IsNotExist(e) {
					return e
				}
				if cache != nil {
					e = cache.Remove(filepath.Join("full-images-v1", v.key+".png"))
					if e != nil && !os.IsNotExist(e) {
						cache.Close()
						return e
					}
					e = cache.Remove(filepath.Join("video-v1", v.key+".mp4"))
					if e != nil && !os.IsNotExist(e) {
						cache.Close()
						return e
					}
					for _, size := range thumbnails.Sizes {
						relative := thumbnails.Path("", v.key, size)
						e = cache.Remove(filepath.FromSlash(relative))
						if e != nil && !os.IsNotExist(e) {
							cache.Close()
							return e
						}
					}
					cache.Close()
				}
			}
			tx, e := s.DB.BeginTx(ctx, nil)
			if e != nil {
				return e
			}
			// Completed upload receipts must not return the ID of a deleted original.
			if _, e = tx.ExecContext(ctx, "DELETE FROM upload_sessions WHERE asset_id=?", v.id); e == nil {
				_, e = tx.ExecContext(ctx, "DELETE FROM assets WHERE id=? AND EXISTS(SELECT 1 FROM media_deletions WHERE asset_id=?)", v.id, v.id)
			}
			if e != nil {
				tx.Rollback()
				return e
			}
			if e = tx.Commit(); e != nil {
				return e
			}
			return nil
		}()
		if deletionErr != nil {
			failures = append(failures, fmt.Errorf("asset %d: %w", v.id, deletionErr))
		}
	}
	return errors.Join(failures...)
}
