package api

import (
	"context"
	"encoding/json"
	"gallery/internal/access"
	"gallery/internal/auth"
	"gallery/internal/config"
	"gallery/internal/database"
	"gallery/internal/places"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestMapPermissionsAndBounds(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "map.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, e = db.Exec(`INSERT INTO libraries VALUES('p','Photos','/photos',1);INSERT INTO assets(library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan,latitude,longitude) VALUES('p','Alice/a.jpg','Alice','a.jpg','.jpg','image/jpeg','image',1,1,'2026','s',52.52,13.405),('p','Bob/b.jpg','Bob','b.jpg','.jpg','image/jpeg','image',1,1,'2026','s',48.8566,2.3522),('p','Alice/e.jpg','Alice','e.jpg','.jpg','image/jpeg','image',1,1,'2026','s',0,179.9),('p','Alice/w.jpg','Alice','w.jpg','.jpg','image/jpeg','image',1,1,'2026','s',0,-179.9),('p','Alice/no.jpg','Alice','no.jpg','.jpg','image/jpeg','image',1,1,'2026','s',NULL,NULL);`)
	if e != nil {
		t.Fatal(e)
	}
	if e = auth.Bootstrap(db, config.Config{Username: "admin"}); e != nil {
		t.Fatal(e)
	}
	db.Exec("INSERT INTO users(username,display_name,password_hash,role) VALUES('alice','Alice','','member'),('bob','Bob','','member')")
	db.Exec("INSERT INTO folder_grants(user_id,library_id,folder,label) VALUES(2,'p','Alice','Mine'),(3,'p','Bob','Mine')")
	if e = places.Backfill(context.Background(), db); e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	(&Server{DB: db}).Register(mux)
	req := func(id int64, path string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		r = r.WithContext(access.WithUser(r.Context(), access.User{ID: id, Role: "member"}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w
	}
	w := req(2, "/api/map", 200)
	var result struct {
		Items                 []struct{ Count int }
		Geotagged, WithoutGPS int
	}
	if e = json.Unmarshal(w.Body.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, p := range result.Items {
		count += p.Count
	}
	if count != 3 || result.Geotagged != 3 || result.WithoutGPS != 1 || strings.Contains(w.Body.String(), "Paris") {
		t.Fatal("map leaked counts or places", w.Body.String())
	}
	for _, path := range []string{"/api/places", "/api/places?q=Paris", "/api/search?q=Paris"} {
		w = req(2, path, 200)
		if strings.Contains(w.Body.String(), "Paris") {
			t.Fatal("private place leaked", path, w.Body.String())
		}
	}
	w = req(2, "/api/places?q=Berlin", 200)
	if !strings.Contains(w.Body.String(), "Berlin") {
		t.Fatal("missing place", w.Body.String())
	}
	var pid string
	db.QueryRow("SELECT place_id FROM assets WHERE id=1").Scan(&pid)
	w = req(3, "/api/assets?place="+pid, 200)
	if !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatal("place ID bypass")
	}
	w = req(2, "/api/map?bbox=170,-5,-170,5&zoom=5", 200)
	result.Items = nil
	json.Unmarshal(w.Body.Bytes(), &result)
	count = 0
	for _, p := range result.Items {
		count += p.Count
	}
	if count != 2 {
		t.Fatal("antimeridian map", w.Body.String())
	}
	w = req(2, "/api/assets?bbox=170,-5,-170,5", 200)
	var page struct{ Items []Asset }
	json.Unmarshal(w.Body.Bytes(), &page)
	if len(page.Items) != 2 {
		t.Fatal("antimeridian photo selection")
	}
	for _, v := range []string{"NaN,0,20,30", "0,0,Inf,30", "0,20,30,10", "-181,0,20,30", "0,0,20,91", "1,2,3"} {
		req(2, "/api/map?bbox="+v, 400)
		req(2, "/api/assets?bbox="+v, 400)
	}
	req(2, "/api/map?zoom=99", 400)
	req(2, "/api/places?offset=-1", 400)
	db.Exec("INSERT INTO albums(name,owner_id) VALUES('Trip',2)")
	db.Exec("INSERT INTO album_assets(album_id,asset_id,added_by) VALUES(1,1,2)")
	db.Exec("INSERT INTO album_members(album_id,user_id,role) VALUES(1,3,'viewer')")
	w = req(3, "/api/places?q=Berlin", 200)
	if !strings.Contains(w.Body.String(), "Berlin") {
		t.Fatal("shared location missing")
	}
	db.Exec("UPDATE album_members SET expires_at=1")
	w = req(3, "/api/places?q=Berlin", 200)
	if strings.Contains(w.Body.String(), "Berlin") {
		t.Fatal("expired shared location leaked")
	}
	db.Exec("UPDATE libraries SET enabled=0")
	w = req(2, "/api/map", 200)
	if !strings.Contains(w.Body.String(), `"geotagged":0`) {
		t.Fatal("disabled library leaked")
	}
}
