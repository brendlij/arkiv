package library

import (
	"context"
	"gallery/internal/config"
	"gallery/internal/database"
	"os"
	"path/filepath"
	"testing"
)

func TestContainment(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("x"), 0600)
	if _, e := Resolve(root, "photo.jpg"); e != nil {
		t.Fatal(e)
	}
	f, e := Open(root, "photo.jpg")
	if e != nil {
		t.Fatal(e)
	}
	f.Close()
	for _, p := range []string{"../secret.jpg", "/etc/passwd", "..\\secret.jpg", "C:\\secret.jpg", "missing.jpg"} {
		if _, e := Resolve(root, p); e == nil {
			t.Errorf("accepted %q", p)
		}
	}
	if e := os.Symlink(filepath.Join(root, "photo.jpg"), filepath.Join(root, "link.jpg")); e == nil {
		if _, e = Resolve(root, "link.jpg"); e == nil {
			t.Fatal("followed symlink")
		}
	}
}

func TestSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.jpg"), []byte("private"), 0600)
	if e := os.Symlink(outside, filepath.Join(root, "escape")); e != nil {
		t.Skipf("host cannot create symlinks: %v", e)
	}
	if _, e := Open(root, "escape/secret.jpg"); e == nil {
		t.Fatal("symlink escape opened")
	}
}

func TestConfiguredLibraries(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	libs := []config.Library{{ID: "a", Name: "A", Root: t.TempDir(), Enabled: true}, {ID: "b", Name: "B", Root: t.TempDir(), Enabled: false}}
	ctx := context.Background()
	if e = Sync(ctx, db, libs); e != nil {
		t.Fatal(e)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM libraries WHERE enabled=1").Scan(&n)
	if n != 1 {
		t.Fatal("enabled libraries")
	}
	if e = Sync(ctx, db, libs[1:]); e != nil {
		t.Fatal(e)
	}
	db.QueryRow("SELECT count(*) FROM libraries WHERE enabled=1").Scan(&n)
	if n != 0 {
		t.Fatal("removed config still enabled")
	}
	libs[1].Root = t.TempDir()
	if e = Sync(ctx, db, libs[1:]); e == nil {
		t.Fatal("root changed under existing ID")
	}
}
