package storage

import (
	"bytes"
	"context"
	"gallery/internal/database"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateUploadsCopiesVerifiesAndRetainsLegacyFiles(t *testing.T) {
	legacy, target := filepath.Join(t.TempDir(), "uploads"), filepath.Join(t.TempDir(), "originals")
	db, err := database.Open(filepath.Join(t.TempDir(), "gallery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("INSERT INTO libraries(id,name,root,enabled) VALUES('arkiv-uploads','Uploads',?,1)", legacy); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"user-2/ab/cd/abcdef.jpg": []byte("original bytes"),
		".incoming/session.part":  []byte("resumable bytes"),
	}
	for rel, data := range files {
		path := filepath.Join(legacy, filepath.FromSlash(rel))
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := MigrateUploads(context.Background(), db, legacy, target)
	if err != nil || result.Copied != len(files) {
		t.Fatalf("migration: %+v %v", result, err)
	}
	for rel, data := range files {
		for _, root := range []string{legacy, target} {
			got, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if readErr != nil || !bytes.Equal(got, data) {
				t.Fatalf("%s not preserved at %s: %v", rel, root, readErr)
			}
		}
	}
	var root string
	if err = db.QueryRow("SELECT root FROM libraries WHERE id='arkiv-uploads'").Scan(&root); err != nil || !samePath(root, target) {
		t.Fatalf("library root not migrated: %q %v", root, err)
	}
	result, err = MigrateUploads(context.Background(), db, legacy, target)
	if err != nil || result.Copied != 0 {
		t.Fatalf("migration not idempotent: %+v %v", result, err)
	}
}

func TestMigrateUploadsRejectsDifferentTargetData(t *testing.T) {
	legacy, target := filepath.Join(t.TempDir(), "uploads"), filepath.Join(t.TempDir(), "originals")
	db, err := database.Open(filepath.Join(t.TempDir(), "gallery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("INSERT INTO libraries(id,name,root,enabled) VALUES('arkiv-uploads','Uploads',?,1)", legacy); err != nil {
		t.Fatal(err)
	}
	for root, data := range map[string][]byte{legacy: []byte("old"), target: []byte("different")} {
		path := filepath.Join(root, "user-1", "photo.jpg")
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = MigrateUploads(context.Background(), db, legacy, target); err == nil {
		t.Fatal("conflicting target data accepted")
	}
	var root string
	db.QueryRow("SELECT root FROM libraries WHERE id='arkiv-uploads'").Scan(&root)
	if !samePath(root, legacy) {
		t.Fatalf("library root changed after failed migration: %q", root)
	}
}

func TestRequireCurrentUploadsRoot(t *testing.T) {
	legacy, target := filepath.Join(t.TempDir(), "uploads"), filepath.Join(t.TempDir(), "originals")
	db, err := database.Open(filepath.Join(t.TempDir(), "gallery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = RequireCurrentUploadsRoot(context.Background(), db, legacy, target); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO libraries(id,name,root,enabled) VALUES('arkiv-uploads','Uploads',?,1)", legacy); err != nil {
		t.Fatal(err)
	}
	if err = RequireCurrentUploadsRoot(context.Background(), db, legacy, target); err == nil {
		t.Fatal("legacy upload root did not require migration")
	}
	if _, err = db.Exec("UPDATE libraries SET root=? WHERE id='arkiv-uploads'", target); err != nil {
		t.Fatal(err)
	}
	if err = RequireCurrentUploadsRoot(context.Background(), db, legacy, target); err != nil {
		t.Fatal(err)
	}
}
