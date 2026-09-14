package database

import (
	"path/filepath"
	"testing"
)

func TestMigrationsPersistence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "gallery.db")
	db, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var n int
	db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&n)
	if n != 12 {
		t.Fatalf("migrations: %d", n)
	}
	var wal string
	db.QueryRow("PRAGMA journal_mode").Scan(&wal)
	if wal != "wal" {
		t.Fatal(wal)
	}
	db.QueryRow("PRAGMA foreign_keys").Scan(&n)
	if n != 1 {
		t.Fatal("foreign keys disabled")
	}
	if _, e = db.Exec("INSERT INTO albums(name) VALUES('Travel')"); e != nil {
		t.Fatal(e)
	}
	db.Close()
	db, e = Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	db.QueryRow("SELECT count(*) FROM albums WHERE name='Travel'").Scan(&n)
	if n != 1 {
		t.Fatal("album not persistent")
	}
	if _, e = db.Exec("INSERT INTO album_assets VALUES(1,123)"); e == nil {
		t.Fatal("foreign key not enforced")
	}
}
