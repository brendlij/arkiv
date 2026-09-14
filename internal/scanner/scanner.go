package scanner

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"gallery/internal/config"
	"gallery/internal/media"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Scanner struct {
	DB            *sql.DB
	Libraries     []config.Library
	Running       atomic.Bool
	Examined      atomic.Int64
	mu            sync.RWMutex
	lastError     string
	lastCompleted string
}

func (s *Scanner) Status() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastCompleted, s.lastError
}

type Result struct {
	Discovered int   `json:"discovered"`
	Updated    int   `json:"updated"`
	Removed    int64 `json:"removed"`
}

func Kind(ext string) (string, string) {
	f, ok := media.Lookup(ext)
	if !ok {
		return "", ""
	}
	return f.Kind, f.MIME
}
func (s *Scanner) Scan(ctx context.Context) (Result, error) {
	if !s.Running.CompareAndSwap(false, true) {
		return Result{}, fmt.Errorf("scan already running")
	}
	defer s.Running.Store(false)
	s.Examined.Store(0)
	var total Result
	var failures []string
	for _, l := range s.Libraries {
		if !l.Enabled {
			continue
		}
		r, e := s.scanRoot(ctx, l)
		total.Discovered += r.Discovered
		total.Updated += r.Updated
		total.Removed += r.Removed
		if e != nil {
			slog.Error("library scan failed; removal reconciliation skipped", "library", l.ID, "error", e)
			failures = append(failures, l.ID+": "+e.Error())
		}
	}
	if len(failures) > 0 {
		s.mu.Lock()
		s.lastError = strings.Join(failures, "; ")
		s.mu.Unlock()
		return total, fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	s.mu.Lock()
	s.lastCompleted = time.Now().UTC().Format(time.RFC3339)
	s.lastError = ""
	s.mu.Unlock()
	return total, nil
}
func (s *Scanner) scanRoot(ctx context.Context, l config.Library) (Result, error) {
	result := Result{}
	start := time.Now()
	slog.Info("library scan started", "library", l.ID)
	info, e := os.Lstat(l.Root)
	if e != nil {
		return result, e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, fmt.Errorf("root must be a real directory")
	}
	tokenBytes := make([]byte, 16)
	if _, e = rand.Read(tokenBytes); e != nil {
		return result, e
	}
	token := hex.EncodeToString(tokenBytes)
	var walkFailure error
	type entry struct {
		path string
		info os.FileInfo
	}
	batch := make([]entry, 0, 128)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		tx, e := s.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		var added, updated int
		for _, item := range batch {
			a, u, e := indexFile(ctx, tx, l, token, item.path, item.info)
			if e != nil {
				return e
			}
			added += a
			updated += u
		}
		if e = tx.Commit(); e != nil {
			return e
		}
		result.Discovered += added
		result.Updated += updated
		batch = batch[:0]
		return nil
	}
	e = filepath.WalkDir(l.Root, func(p string, d fs.DirEntry, walkErr error) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if walkErr != nil {
			walkFailure = walkErr
			slog.Warn("cannot read path", "path", p, "error", walkErr)
			return nil
		}
		if d.IsDir() || d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		kind, _ := Kind(filepath.Ext(p))
		if kind == "" {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			walkFailure = e
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		s.Examined.Add(1)
		batch = append(batch, entry{path: p, info: info})
		if len(batch) == cap(batch) {
			return flush()
		}
		return nil
	})
	if e != nil {
		return result, e
	}
	if e = flush(); e != nil {
		return result, e
	}
	if walkFailure != nil {
		return result, walkFailure
	}
	// A changed/unmounted root must never cause mass deletion during reconciliation.
	after, e := os.Stat(l.Root)
	if e != nil || !os.SameFile(info, after) {
		return result, fmt.Errorf("library root changed during scan")
	}
	r, e := s.DB.ExecContext(ctx, "DELETE FROM assets WHERE library_id=? AND seen_scan<>?", l.ID, token)
	if e != nil {
		return result, e
	}
	result.Removed, e = r.RowsAffected()
	slog.Info("library scan completed", "library", l.ID, "discovered", result.Discovered, "updated", result.Updated, "removed", result.Removed, "elapsed", time.Since(start))
	return result, e
}

func indexFile(ctx context.Context, tx *sql.Tx, l config.Library, token, path string, info os.FileInfo) (int, int, error) {
	rel, e := filepath.Rel(l.Root, path)
	if e != nil {
		return 0, 0, e
	}
	rel = filepath.ToSlash(rel)
	folder := filepath.ToSlash(filepath.Dir(rel))
	if folder == "." {
		folder = ""
	}
	kind, mime := Kind(filepath.Ext(path))
	var id, size, mtime int64
	e = tx.QueryRowContext(ctx, "SELECT id,file_size,modified_at FROM assets WHERE library_id=? AND relative_path=?", l.ID, rel).Scan(&id, &size, &mtime)
	if e != nil && e != sql.ErrNoRows {
		return 0, 0, e
	}
	if e == nil && size == info.Size() && mtime == info.ModTime().UnixNano() {
		_, e = tx.ExecContext(ctx, "UPDATE assets SET seen_scan=? WHERE id=?", token, id)
		return 0, 0, e
	}
	added, updated := 0, 0
	if id == 0 {
		r, e := tx.ExecContext(ctx, "INSERT INTO assets(library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan) VALUES(?,?,?,?,?,?,?,?,?,?,?)", l.ID, rel, folder, info.Name(), strings.ToLower(filepath.Ext(path)), mime, kind, info.Size(), info.ModTime().UnixNano(), info.ModTime().UTC().Format(time.RFC3339), token)
		if e != nil {
			return 0, 0, e
		}
		id, e = r.LastInsertId()
		if e != nil {
			return 0, 0, e
		}
		added = 1
	} else {
		_, e = tx.ExecContext(ctx, "UPDATE assets SET file_size=?,modified_at=?,taken_at=?,seen_scan=?,generation=generation+1,preview_status='pending',metadata_status='pending',preview_key='',error='',indexed_at=CURRENT_TIMESTAMP WHERE id=?", info.Size(), info.ModTime().UnixNano(), info.ModTime().UTC().Format(time.RFC3339), token, id)
		if e != nil {
			return 0, 0, e
		}
		updated = 1
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO jobs(asset_id) VALUES(?) ON CONFLICT(asset_id) DO UPDATE SET state='pending',attempts=0,next_at=0", id)
	return added, updated, e
}
