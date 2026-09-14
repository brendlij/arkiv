package scanner

import (
	"context"
	"errors"
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
	recycle := filepath.Join(root, "#recycle")
	if e = os.Mkdir(recycle, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(recycle, "deleted.jpg"), []byte("ignored"), 0600); e != nil {
		t.Fatal(e)
	}
	managed := filepath.Join(root, "originals")
	if e = os.Mkdir(managed, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(managed, "managed.jpg"), []byte("indexed by uploads"), 0600); e != nil {
		t.Fatal(e)
	}
	s.ExcludedRoots = []string{managed}
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

func TestWalkErrors(t *testing.T) {
	permission := &os.PathError{Op: "readdir", Path: "#recycle", Err: os.ErrPermission}
	if e := handleWalkError("#recycle", permission); e != nil {
		t.Fatalf("permission error remained fatal: %v", e)
	}
	fatal := errors.New("filesystem failure")
	if e := handleWalkError("photos", fatal); !errors.Is(e, fatal) {
		t.Fatalf("fatal error was ignored: %v", e)
	}
}

func TestManagedUploadRootIsNotScannedTwice(t *testing.T) {
	ctx := context.Background()
	photos := t.TempDir()
	managed := filepath.Join(photos, "originals")
	managedFile := filepath.Join(managed, "user-2", "ab", "cd", "abcdef.jpg")
	if e := os.MkdirAll(filepath.Dir(managedFile), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(managedFile, []byte("managed original"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(photos, "manual.jpg"), []byte("manual original"), 0600); e != nil {
		t.Fatal(e)
	}
	db, e := database.Open(filepath.Join(t.TempDir(), "gallery.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	libraries := []config.Library{
		{ID: "photos", Name: "Photos", Root: photos, Enabled: true},
		{ID: "arkiv-uploads", Name: "Uploads", Root: managed, Enabled: true},
	}
	if e = library.Sync(ctx, db, libraries); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec("INSERT INTO assets(library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan) VALUES('arkiv-uploads','user-2/ab/cd/abcdef.jpg','user-2','original.jpg','.jpg','image/jpeg','image',16,1,'2026-01-01','upload')"); e != nil {
		t.Fatal(e)
	}
	s := Scanner{DB: db, Libraries: libraries[:1], ExcludedRoots: []string{managed}}
	result, e := s.Scan(ctx)
	if e != nil || result.Discovered != 1 {
		t.Fatalf("scan result: %+v %v", result, e)
	}
	var total, managedCount int
	db.QueryRow("SELECT count(*) FROM assets").Scan(&total)
	db.QueryRow("SELECT count(*) FROM assets WHERE library_id='arkiv-uploads'").Scan(&managedCount)
	if total != 2 || managedCount != 1 {
		t.Fatalf("managed upload duplicated: total=%d managed=%d", total, managedCount)
	}
}
