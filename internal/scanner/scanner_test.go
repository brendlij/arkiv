package scanner

import (
	"context"
	"gallery/internal/config"
	"gallery/internal/database"
	"gallery/internal/library"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLifecycle(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db, e := database.Open(filepath.Join(t.TempDir(), "gallery.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	libs := []config.Library{{ID: "one", Name: "One", Root: root, Enabled: true}}
	if e = library.Sync(ctx, db, libs); e != nil {
		t.Fatal(e)
	}
	s := Scanner{DB: db, Libraries: libs}
	path := filepath.Join(root, "corrupt.jpg")
	os.WriteFile(path, []byte("not an image"), 0600)
	os.WriteFile(filepath.Join(root, "ignore.txt"), []byte("ignored"), 0600)
	r, e := s.Scan(ctx)
	if e != nil || r.Discovered != 1 {
		t.Fatalf("new: %+v %v", r, e)
	}
	r, e = s.Scan(ctx)
	if e != nil || r.Discovered != 0 || r.Updated != 0 {
		t.Fatalf("unchanged: %+v %v", r, e)
	}
	if _, e = db.Exec("UPDATE assets SET favorite=1"); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(path, []byte("modified corrupt image"), 0600)
	now := time.Now().Add(time.Second)
	os.Chtimes(path, now, now)
	r, e = s.Scan(ctx)
	if e != nil || r.Updated != 1 {
		t.Fatalf("modified: %+v %v", r, e)
	}
	var favorite, generation int
	db.QueryRow("SELECT favorite,generation FROM assets").Scan(&favorite, &generation)
	if favorite != 1 || generation != 2 {
		t.Fatal("update lost user state or generation")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	os.Remove(path)
	if _, e = s.Scan(cancelled); e == nil {
		t.Fatal("cancelled scan succeeded")
	}
	var n int
	db.QueryRow("SELECT count(*) FROM assets").Scan(&n)
	if n != 1 {
		t.Fatal("cancelled scan removed metadata")
	}
	s.Libraries[0].Root = filepath.Join(root, "unavailable")
	if _, e = s.Scan(ctx); e == nil {
		t.Fatal("missing root succeeded")
	}
	db.QueryRow("SELECT count(*) FROM assets").Scan(&n)
	if n != 1 {
		t.Fatal("unavailable root removed metadata")
	}
	s.Libraries[0].Root = root
	r, e = s.Scan(ctx)
	if e != nil || r.Removed != 1 {
		t.Fatalf("deleted: %+v %v", r, e)
	}
	db.QueryRow("SELECT count(*) FROM jobs").Scan(&n)
	if n != 0 {
		t.Fatal("orphan job")
	}
}
