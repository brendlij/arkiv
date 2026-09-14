package api

import (
	"gallery/internal/access"
	"gallery/internal/database"
	"gallery/internal/thumbnails"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFullImageCacheRequiresCurrentAccess(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	for _, q := range []string{
		"INSERT INTO users(id,username,display_name,password_hash,role) VALUES(1,'admin','Admin','','admin'),(2,'viewer','Viewer','','member')",
		"INSERT INTO libraries VALUES('photos','Photos','unused',1)",
		"INSERT INTO assets(id,library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan) VALUES(1,'photos','a.tiff','','a.tiff','.tiff','image/tiff','image',10,1,'2026-01-01','scan')",
	} {
		if _, e = db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	cache := t.TempDir()
	path := filepath.Join(cache, "full-images-v1", thumbnails.Key(1, 1, 10, 1)+".png")
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, []byte("cached full image"), 0600); e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	(&Server{DB: db, Cache: cache}).Register(mux)
	req := func(u access.User, want int) {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/assets/1/full-image", nil)
		r = r.WithContext(access.WithUser(r.Context(), u))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatal(w.Code, w.Body.String())
		}
		if want == 200 && (w.Body.String() != "cached full image" || w.Header().Get("Cache-Control") != "no-store") {
			t.Fatal(w.Body.String())
		}
	}
	req(access.User{ID: 1, Role: "admin"}, 200)
	req(access.User{ID: 2, Role: "member"}, 404)
	if _, e = db.Exec("INSERT INTO folder_grants(user_id,library_id,folder,label) VALUES(2,'photos','','Photos')"); e != nil {
		t.Fatal(e)
	}
	req(access.User{ID: 2, Role: "member"}, 200)
	if _, e = db.Exec("DELETE FROM folder_grants WHERE user_id=2"); e != nil {
		t.Fatal(e)
	}
	req(access.User{ID: 2, Role: "member"}, 404)
	if _, e = db.Exec("UPDATE assets SET trashed_at=1"); e != nil {
		t.Fatal(e)
	}
	req(access.User{ID: 1, Role: "admin"}, 404)
}
