package api

import (
	"gallery/internal/access"
	"gallery/internal/database"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTimeline100K(t *testing.T) {
	if os.Getenv("ARKIV_SCALE_TEST") != "1" {
		t.Skip("set ARKIV_SCALE_TEST=1 for 100,000-row query verification")
	}
	db, e := database.Open(filepath.Join(t.TempDir(), "scale.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, e = db.Exec("INSERT INTO libraries VALUES('scale','Scale','/photos',1)")
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<100000) INSERT INTO assets(library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan) SELECT 'scale','test/'||n||'.jpg','test',n||'.jpg','.jpg','image/jpeg','image',100,1,strftime('%Y-%m-%dT%H:%M:%SZ',1700000000+n,'unixepoch'),'scale' FROM seq`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec("UPDATE assets SET latitude=(id%160)-80,longitude=(id%360)-180"); e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	(&Server{DB: db}).Register(mux)
	for _, path := range []string{"/api/assets?limit=120", "/api/search?q=test&limit=120", "/api/folders?library=scale", "/api/stats", "/api/map?zoom=2", "/api/map?bbox=10,50,15,55&zoom=12"} {
		start := time.Now()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", path, nil)
		mux.ServeHTTP(w, r.WithContext(access.WithUser(r.Context(), access.User{ID: 1, Role: "admin"})))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		t.Logf("100k assets: %s took %s; response %d bytes", path, time.Since(start), w.Body.Len())
	}
}
