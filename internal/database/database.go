package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sort"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000", "CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY)"} {
		if _, err = db.Exec(q); err != nil {
			db.Close()
			return nil, err
		}
	}
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		db.Close()
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
	for _, f := range files {
		var count int
		if err = db.QueryRow("SELECT count(*) FROM schema_migrations WHERE name=?", f.Name()).Scan(&count); err != nil {
			db.Close()
			return nil, err
		}
		if count > 0 {
			continue
		}
		body, e := migrations.ReadFile("migrations/" + f.Name())
		if e != nil {
			return nil, e
		}
		tx, e := db.BeginTx(context.Background(), nil)
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(string(body)); e == nil {
			_, e = tx.Exec("INSERT INTO schema_migrations VALUES (?)", f.Name())
		}
		if e != nil {
			tx.Rollback()
			db.Close()
			return nil, fmt.Errorf("migration %s: %w", f.Name(), e)
		}
		if e = tx.Commit(); e != nil {
			db.Close()
			return nil, e
		}
	}
	return db, nil
}
