package scanner

import (
	"context"
	"fmt"
	"gallery/internal/config"
	"gallery/internal/database"
	"gallery/internal/library"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanTenThousand(t *testing.T) {
	if os.Getenv("ARKIV_SCALE_TEST") != "1" {
		t.Skip("set ARKIV_SCALE_TEST=1")
	}
	root := t.TempDir()
	for i := 0; i < 10000; i++ {
		if e := os.WriteFile(filepath.Join(root, fmt.Sprintf("%05d.jpg", i)), []byte("fixture"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	db, e := database.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	libs := []config.Library{{ID: "scale", Name: "Scale", Root: root, Enabled: true}}
	if e = library.Sync(context.Background(), db, libs); e != nil {
		t.Fatal(e)
	}
	s := Scanner{DB: db, Libraries: libs}
	start := time.Now()
	r, e := s.Scan(context.Background())
	if e != nil || r.Discovered != 10000 {
		t.Fatalf("%+v %v", r, e)
	}
	t.Log("10k new files", time.Since(start))
	start = time.Now()
	r, e = s.Scan(context.Background())
	if e != nil || r.Discovered != 0 || r.Updated != 0 || r.Removed != 0 {
		t.Fatalf("%+v %v", r, e)
	}
	t.Log("10k unchanged files", time.Since(start))
}
