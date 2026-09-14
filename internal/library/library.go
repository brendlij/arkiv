package library

import (
	"context"
	"database/sql"
	"fmt"
	"gallery/internal/config"
	"os"
	"path/filepath"
	"strings"
)

func Sync(ctx context.Context, db *sql.DB, libs []config.Library) error {
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "UPDATE libraries SET enabled=0"); e != nil {
		return e
	}
	for _, l := range libs {
		var root string
		e = tx.QueryRowContext(ctx, "SELECT root FROM libraries WHERE id=?", l.ID).Scan(&root)
		if e != nil && e != sql.ErrNoRows {
			return e
		}
		if root != "" && root != l.Root {
			return fmt.Errorf("library %s root changed; use a new ID to preserve associations", l.ID)
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO libraries(id,name,root,enabled) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,enabled=excluded.enabled", l.ID, l.Name, l.Root, l.Enabled); e != nil {
			return e
		}
	}
	return tx.Commit()
}

// Resolve rejects every symlink component, including the root. Original mounts are never written.
func Resolve(root, rel string) (string, error) {
	if !filepath.IsLocal(rel) || strings.Contains(rel, "\\") || strings.Contains(rel, ":") {
		return "", fmt.Errorf("invalid relative path")
	}
	base, e := filepath.Abs(root)
	if e != nil {
		return "", e
	}
	resolved, e := filepath.EvalSymlinks(base)
	if e != nil {
		return "", e
	}
	if !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(base)) {
		return "", fmt.Errorf("symlink library root")
	}
	p := base
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		p = filepath.Join(p, part)
		info, e := os.Lstat(p)
		if e != nil {
			return "", e
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlinks are not indexed")
		}
	}
	r, e := filepath.Rel(base, p)
	if e != nil || !filepath.IsLocal(r) {
		return "", fmt.Errorf("path outside library")
	}
	return p, nil
}

// Open pins the root and lets the OS reject traversal even if a path component
// changes after Resolve's symlink policy check.
func Open(root, rel string) (*os.File, error) {
	if _, e := Resolve(root, rel); e != nil {
		return nil, e
	}
	r, e := os.OpenRoot(root)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	return r.Open(filepath.FromSlash(rel))
}
