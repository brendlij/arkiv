package api

import (
	"context"
	"encoding/json"
	"fmt"
	"gallery/internal/access"
	"gallery/internal/config"
	"gallery/internal/database"
	"gallery/internal/library"
	"gallery/internal/scanner"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessingDashboardAndTargetedRetry(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "gallery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := t.TempDir()
	libs := []config.Library{{ID: "photos", Name: "Photos", Root: root, Enabled: true}}
	if err = library.Sync(context.Background(), db, libs); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 55; i++ {
		if err = os.WriteFile(filepath.Join(root, fmt.Sprintf("%02d.jpg", i)), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	scan := &scanner.Scanner{DB: db, Libraries: libs}
	if _, err = scan.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"UPDATE assets SET preview_status='ready',metadata_status='ready' WHERE id=1",
		"DELETE FROM jobs WHERE asset_id=1",
		"UPDATE jobs SET state='failed',attempts=3 WHERE asset_id IN (2,3)",
		"UPDATE assets SET preview_status='failed',metadata_status='ready',error='broken media' WHERE id IN (2,3)",
		"UPDATE jobs SET state='running',stage='previews' WHERE asset_id=4",
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	(&Server{DB: db, Scanner: scan}).Register(mux)
	req := func(role, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r = r.WithContext(access.WithUser(r.Context(), access.User{ID: 1, Role: role}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, method := range []string{"GET", "POST"} {
		path := "/api/processing"
		if method == "POST" {
			path += "/retry"
		}
		if w := req("member", method, path, `{"id":0}`); w.Code != 403 {
			t.Fatal("member access", w.Code)
		}
	}
	w := req("admin", "GET", "/api/processing", "")
	var snapshot struct {
		Total, MetadataReady, PreviewsReady, ScanExamined int
		Counts                                            map[string]int
		Items                                             []struct {
			ID           int
			Stage, Error string
		}
	}
	if err = json.Unmarshal(w.Body.Bytes(), &snapshot); err != nil || w.Code != 200 {
		t.Fatal(w.Body.String(), err)
	}
	if snapshot.Total != 55 || snapshot.MetadataReady != 3 || snapshot.PreviewsReady != 1 || snapshot.ScanExamined != 55 || snapshot.Counts["failed"] != 2 || snapshot.Counts["running"] != 1 || snapshot.Counts["pending"] != 51 || len(snapshot.Items) != 2 || snapshot.Items[0].Error != "broken media" {
		t.Fatal(w.Body.String())
	}
	for _, path := range []string{"/api/processing?state=pending", "/api/processing?state=pending&offset=50", "/api/processing?state=running"} {
		w = req("admin", "GET", path, "")
		if err = json.Unmarshal(w.Body.Bytes(), &snapshot); err != nil {
			t.Fatal(err)
		}
		want := 1
		if path == "/api/processing?state=pending" {
			want = 50
		}
		if len(snapshot.Items) != want {
			t.Fatal(path, w.Body.String())
		}
		if strings.Contains(path, "running") && snapshot.Items[0].Stage != "previews" {
			t.Fatal("missing phase")
		}
	}
	for _, path := range []string{"/api/processing?state=oops", "/api/processing?offset=-1", "/api/processing?offset=oops"} {
		if w = req("admin", "GET", path, ""); w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
	for _, body := range []string{`{"id":2}`, `{"id":0}`} {
		w = req("admin", "POST", "/api/processing/retry", body)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"retried":1`) {
			t.Fatal(w.Body.String())
		}
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM jobs WHERE asset_id IN (2,3) AND state='pending' AND attempts=0 AND next_at=0").Scan(&n); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if err = db.QueryRow("SELECT count(*) FROM jobs WHERE asset_id=4 AND state='running'").Scan(&n); err != nil || n != 1 {
		t.Fatal("running job changed", err)
	}
	if err = db.QueryRow("SELECT count(*) FROM jobs WHERE asset_id=1").Scan(&n); err != nil || n != 0 {
		t.Fatal("healthy file requeued", err)
	}
	if _, err = db.Exec("UPDATE jobs SET state='failed' WHERE asset_id=2; UPDATE libraries SET enabled=0"); err != nil {
		t.Fatal(err)
	}
	w = req("admin", "POST", "/api/processing/retry", `{"id":0}`)
	if !strings.Contains(w.Body.String(), `"retried":0`) {
		t.Fatal("disabled library retried", w.Body.String())
	}
	w = req("admin", "GET", "/api/processing", "")
	if err = json.Unmarshal(w.Body.Bytes(), &snapshot); err != nil || snapshot.Total != 0 || len(snapshot.Items) != 0 {
		t.Fatal("disabled library shown", w.Body.String())
	}
}
