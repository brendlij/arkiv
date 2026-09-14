package api

import (
	"context"
	"encoding/json"
	"fmt"
	"gallery/internal/access"
	"gallery/internal/auth"
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

func TestAPICollectionsAndPagination(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "gallery.db")
	db, e := database.Open(dbPath)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { db.Close() }()
	libs := []config.Library{{ID: "photos", Name: "Photos", Root: root, Enabled: true}}
	if e = library.Sync(context.Background(), db, libs); e != nil {
		t.Fatal(e)
	}
	os.MkdirAll(filepath.Join(root, "Travel", "Paris"), 0700)
	os.MkdirAll(filepath.Join(root, "inbox"), 0700)
	for _, p := range []string{"a.jpg", "Travel/b.jpg", "Travel/Paris/c.jpg", "inbox/d.mp4"} {
		if e = os.WriteFile(filepath.Join(root, p), []byte("original-data"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	scan := &scanner.Scanner{DB: db, Libraries: libs}
	if _, e = scan.Scan(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = auth.Bootstrap(db, config.Config{Username: "admin"}); e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	s := &Server{DB: db, Cache: t.TempDir(), Scanner: scan}
	s.Register(mux)
	req := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r = r.WithContext(access.WithUser(r.Context(), access.User{ID: 1, Role: "admin"}))
		mux.ServeHTTP(w, r)
		if w.Code >= 500 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w
	}
	first := req("GET", "/api/assets?limit=2", "")
	if first.Code != 200 {
		t.Fatal(first.Code)
	}
	var p struct {
		Items []Asset `json:"items"`
		Next  string  `json:"nextCursor"`
	}
	if e = json.Unmarshal(first.Body.Bytes(), &p); e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 2 || p.Next == "" {
		t.Fatal(first.Body.String())
	}
	firstID := p.Items[0].ID
	w := req("GET", "/api/assets?limit=2&cursor="+p.Next, "")
	var p2 struct {
		Items []Asset `json:"items"`
		Next  string  `json:"nextCursor"`
	}
	json.Unmarshal(w.Body.Bytes(), &p2)
	if len(p2.Items) != 2 || p2.Next != "" || p2.Items[0].ID == firstID {
		t.Fatal("pagination incorrect")
	}
	for _, path := range []string{"/api/assets?limit=0", "/api/assets?cursor=bad", "/api/assets/abc", "/api/assets/1/thumbnail/999"} {
		if w = req("GET", path, ""); w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
	for _, path := range []string{"/api/assets/999", "/api/assets/999/original"} {
		if w = req("GET", path, ""); w.Code != 404 {
			t.Fatal(path, w.Code)
		}
	}
	w = req("POST", fmt.Sprintf("/api/assets/%d/favorite", firstID), `{"favorite":true}`)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = req("POST", "/api/albums", `{"name":"Summer"}`)
	if w.Code != 201 {
		t.Fatal(w.Code)
	}
	var al album
	json.Unmarshal(w.Body.Bytes(), &al)
	for i := 0; i < 2; i++ {
		w = req("POST", fmt.Sprintf("/api/albums/%d/assets", al.ID), fmt.Sprintf(`{"assetId":%d}`, firstID))
		if w.Code != 204 {
			t.Fatal(w.Code)
		}
	}
	w = req("GET", fmt.Sprintf("/api/assets?album=%d", al.ID), "")
	if !strings.Contains(w.Body.String(), `"favorite":true`) {
		t.Fatal(w.Body.String())
	}
	for _, path := range []string{"/api/search?q=Travel", "/api/search?q=2026", "/api/search?q=%22OR%22", "/api/assets?inbox=true", "/api/folders?library=photos", "/api/folders?library=photos&parent=Travel", "/api/stats"} {
		if w = req("GET", path, ""); w.Code != 200 {
			t.Fatal(path, w.Code)
		}
	}
	w = req("GET", "/api/folders?library=photos&parent=Travel", "")
	if !strings.Contains(w.Body.String(), `"path":"Travel/Paris"`) {
		t.Fatal(w.Body.String())
	}
	var id int64
	db.QueryRow("SELECT id FROM assets WHERE relative_path='a.jpg'").Scan(&id)
	r := httptest.NewRequest("GET", fmt.Sprintf("/api/assets/%d/original", id), nil)
	r.Header.Set("Range", "bytes=0-3")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r.WithContext(access.WithUser(r.Context(), access.User{ID: 1, Role: "admin"})))
	if w.Code != 206 || w.Body.String() != "orig" {
		t.Fatal("range", w.Code, w.Body.String())
	}
	os.Remove(filepath.Join(root, "a.jpg"))
	if w = req("GET", fmt.Sprintf("/api/assets/%d/original", id), ""); w.Code != 404 {
		t.Fatal("missing file")
	}
	if _, e = db.Exec("UPDATE assets SET relative_path='../outside.jpg' WHERE id=?", id); e != nil {
		t.Fatal(e)
	}
	if w = req("GET", fmt.Sprintf("/api/assets/%d/original", id), ""); w.Code != 404 {
		t.Fatal("traversal")
	}
	db.Close()
	db, e = database.Open(dbPath)
	if e != nil {
		t.Fatal(e)
	}
	var count int
	db.QueryRow("SELECT count(*) FROM user_favorites WHERE user_id=1").Scan(&count)
	if count != 1 {
		t.Fatal("favorite restart persistence")
	}
	db.QueryRow("SELECT count(*) FROM album_assets").Scan(&count)
	if count != 1 {
		t.Fatal("album restart persistence")
	}
}
