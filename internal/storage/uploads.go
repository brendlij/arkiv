package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type MigrationResult struct {
	Copied  int
	Skipped int
}

func RequireCurrentUploadsRoot(ctx context.Context, db *sql.DB, legacy, target string) error {
	var current string
	err := db.QueryRowContext(ctx, "SELECT root FROM libraries WHERE id='arkiv-uploads'").Scan(&current)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && samePath(current, target)) {
		return nil
	}
	if err != nil {
		return err
	}
	if samePath(current, legacy) {
		return fmt.Errorf("legacy managed uploads detected at %s; stop Arkiv and run `arkiv migrate-uploads` with ARKIV_UPLOAD_DIR=%s", legacy, target)
	}
	return fmt.Errorf("managed upload library uses unexpected root %q; configured ARKIV_UPLOAD_DIR is %q", current, target)
}

func MigrateUploads(ctx context.Context, db *sql.DB, legacy, target string) (MigrationResult, error) {
	var result MigrationResult
	legacy = filepath.Clean(legacy)
	target = filepath.Clean(target)
	if !filepath.IsAbs(legacy) || !filepath.IsAbs(target) || rootsOverlap(legacy, target) {
		return result, fmt.Errorf("legacy and target upload directories must be distinct absolute paths")
	}
	var current string
	if err := db.QueryRowContext(ctx, "SELECT root FROM libraries WHERE id='arkiv-uploads'").Scan(&current); errors.Is(err, sql.ErrNoRows) {
		slog.Info("no legacy managed upload library found; migration not needed")
		return result, nil
	} else if err != nil {
		return result, err
	}
	if samePath(current, target) {
		slog.Info("managed upload storage already uses target", "target", target)
		return result, nil
	}
	if !samePath(current, legacy) {
		return result, fmt.Errorf("managed upload library uses unexpected root %q; expected legacy root %q", current, legacy)
	}
	if info, err := os.Lstat(legacy); err != nil {
		return result, fmt.Errorf("open legacy uploads: %w", err)
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, fmt.Errorf("legacy upload root must be a real directory")
	}
	if err := os.MkdirAll(target, 0700); err != nil {
		return result, fmt.Errorf("create target uploads: %w", err)
	}
	if info, err := os.Lstat(target); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, fmt.Errorf("target upload root must be a real directory")
	}
	slog.Info("copying legacy managed uploads", "source", legacy, "target", target)
	err := filepath.WalkDir(legacy, func(source string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if source == legacy {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("legacy uploads contain symlink %q", source)
		}
		rel, err := filepath.Rel(legacy, source)
		if err != nil || !filepath.IsLocal(rel) {
			return fmt.Errorf("invalid legacy upload path %q", source)
		}
		destination := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported file in legacy uploads: %q", source)
		}
		copied, err := copyVerified(source, destination)
		if err != nil {
			return err
		}
		if copied {
			result.Copied++
			slog.Info("copied legacy upload", "path", rel)
		} else {
			result.Skipped++
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("migrate uploads: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	update, err := tx.ExecContext(ctx, "UPDATE libraries SET root=? WHERE id='arkiv-uploads' AND root=?", target, current)
	if err != nil {
		return result, err
	}
	if changed, err := update.RowsAffected(); err != nil || changed != 1 {
		return result, fmt.Errorf("managed upload library changed during migration")
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	slog.Info("legacy managed uploads migrated; source files retained", "copied", result.Copied, "existing", result.Skipped, "source", legacy, "target", target)
	return result, nil
}

func copyVerified(source, destination string) (bool, error) {
	if info, err := os.Lstat(destination); err == nil {
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("migration target collision at %q", destination)
		}
		equal, err := equalFiles(source, destination)
		if err != nil {
			return false, err
		}
		if !equal {
			return false, fmt.Errorf("migration target contains different data at %q", destination)
		}
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return false, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".arkiv-migrate-*.part")
	if err != nil {
		return false, err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	sourceFile, err := os.Open(source)
	if err != nil {
		temporary.Close()
		return false, err
	}
	_, copyErr := io.Copy(temporary, sourceFile)
	sourceFile.Close()
	if copyErr == nil {
		copyErr = temporary.Sync()
	}
	if closeErr := temporary.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return false, copyErr
	}
	equal, err := equalFiles(source, temporaryName)
	if err != nil || !equal {
		if err == nil {
			err = fmt.Errorf("copied data verification failed for %q", source)
		}
		return false, err
	}
	if err = os.Rename(temporaryName, destination); err != nil {
		return false, err
	}
	return true, nil
}

func equalFiles(a, b string) (bool, error) {
	first, err := os.Open(a)
	if err != nil {
		return false, err
	}
	defer first.Close()
	second, err := os.Open(b)
	if err != nil {
		return false, err
	}
	defer second.Close()
	firstHash, secondHash := sha256.New(), sha256.New()
	if _, err = io.Copy(firstHash, first); err != nil {
		return false, err
	}
	if _, err = io.Copy(secondHash, second); err != nil {
		return false, err
	}
	return string(firstHash.Sum(nil)) == string(secondHash.Sum(nil)), nil
}

func rootsOverlap(a, b string) bool {
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		if rel, err := filepath.Rel(pair[0], pair[1]); err == nil && filepath.IsLocal(rel) {
			return true
		}
	}
	return false
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
