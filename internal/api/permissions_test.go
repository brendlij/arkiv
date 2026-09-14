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
	"gallery/internal/thumbnails"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMultiUserPermissions(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "gallery.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	root := t.TempDir()
	libs := []config.Library{{ID: "photos", Name: "Photos", Root: root, Enabled: true}}
	if e = library.Sync(context.Background(), db, libs); e != nil {
		t.Fatal(e)
	}
	for _, folder := range []string{"Alice", "Alice/sub", "Alice-other", "Bob"} {
		os.MkdirAll(filepath.Join(root, folder), 0700)
		os.WriteFile(filepath.Join(root, folder, "photo.jpg"), []byte("private-original"), 0600)
	}
	scan := scanner.Scanner{DB: db, Libraries: libs}
	if _, e = scan.Scan(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = auth.Bootstrap(db, config.Config{Username: "admin"}); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"alice", "bob", "carol"} {
		if _, e = db.Exec("INSERT INTO users(username,display_name,password_hash,role) VALUES(?,?,?,'member')", name, name, ""); e != nil {
			t.Fatal(e)
		}
	}
	admin := access.User{ID: 1, Role: "admin"}
	alice := access.User{ID: 2, Role: "member"}
	bob := access.User{ID: 3, Role: "member"}
	carol := access.User{ID: 4, Role: "member"}
	mux := http.NewServeMux()
	cache := t.TempDir()
	(&Server{DB: db, Cache: cache}).Register(mux)
	req := func(u access.User, method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r = r.WithContext(access.WithUser(r.Context(), u))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("user %d %s %s = %d want %d: %s", u.ID, method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	req(admin, "POST", "/api/admin/users/2/folders", `{"libraryId":"photos","folder":"Alice","label":"My photos"}`, 204)
	req(admin, "POST", "/api/admin/users/3/folders", `{"libraryId":"photos","folder":"Bob","label":"My photos"}`, 204)
	for _, folder := range []string{"../Bob", "Alice/../Bob", "/Bob", "Alice\\\\sub", "Alice/", "."} {
		b, _ := json.Marshal(map[string]string{"libraryId": "photos", "folder": folder, "label": "bad"})
		req(admin, "POST", "/api/admin/users/2/folders", string(b), 400)
	}
	var aid, bid int64
	db.QueryRow("SELECT id FROM assets WHERE folder='Alice'").Scan(&aid)
	db.QueryRow("SELECT id FROM assets WHERE folder='Bob'").Scan(&bid)
	key := strings.Repeat("a", 64)
	preview := thumbnails.Path(cache, key, 256)
	os.MkdirAll(filepath.Dir(preview), 0700)
	os.WriteFile(preview, []byte("cached-preview"), 0600)
	db.Exec("UPDATE assets SET preview_status='ready',preview_key=? WHERE id=?", key, aid)
	asset := fmt.Sprintf("/api/assets/%d", aid)
	for _, endpoint := range []string{asset, asset + "/original", asset + "/thumbnail/256"} {
		req(bob, "GET", endpoint, "", 404)
	}
	w := req(alice, "GET", "/api/assets", "", 200)
	var page struct{ Items []Asset }
	json.Unmarshal(w.Body.Bytes(), &page)
	if len(page.Items) != 2 {
		t.Fatal("grant must include children but not Alice-other", w.Body.String())
	}
	w = req(bob, "GET", "/api/search?q=Alice", "", 200)
	if !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatal("search leaked", w.Body.String())
	}
	w = req(bob, "GET", "/api/folders?library=photos", "", 200)
	if strings.Contains(w.Body.String(), "Alice") {
		t.Fatal("folders leaked")
	}
	req(alice, "POST", asset+"/favorite", `{"favorite":true}`, 200)
	req(bob, "POST", asset+"/favorite", `{"favorite":true}`, 404)
	w = req(alice, "POST", "/api/albums", `{"name":"Private story"}`, 201)
	var al album
	json.Unmarshal(w.Body.Bytes(), &al)
	ap := fmt.Sprintf("/api/albums/%d", al.ID)
	req(alice, "POST", ap+"/assets", fmt.Sprintf(`{"assetId":%d}`, aid), 204)
	req(bob, "GET", ap, "", 404)
	req(bob, "GET", fmt.Sprintf("/api/assets?album=%d", al.ID), "", 404)
	req(alice, "POST", ap+"/members", `{"username":"bob","role":"viewer"}`, 204)
	req(bob, "GET", asset, "", 200)
	thumb := req(bob, "GET", asset+"/thumbnail/256", "", 200)
	if thumb.Body.String() != "cached-preview" || thumb.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("shared thumbnail caching")
	}
	w = req(bob, "GET", asset+"/original", "", 200)
	if w.Body.String() != "private-original" {
		t.Fatal("shared original missing")
	}
	w = req(bob, "GET", asset, "", 200)
	if strings.Contains(w.Body.String(), `"favorite":true`) {
		t.Fatal("favorites leaked")
	}
	req(bob, "POST", ap+"/assets", fmt.Sprintf(`{"assetId":%d}`, bid), 403)
	req(bob, "POST", ap+"/members", `{"username":"carol","role":"viewer"}`, 403)
	w = req(bob, "POST", "/api/albums", `{"name":"Bob story"}`, 201)
	var bal album
	json.Unmarshal(w.Body.Bytes(), &bal)
	req(bob, "POST", fmt.Sprintf("/api/albums/%d/assets", bal.ID), fmt.Sprintf(`{"assetId":%d}`, aid), 403)
	req(alice, "POST", ap+"/members", `{"username":"bob","role":"contributor"}`, 204)
	req(bob, "POST", ap+"/assets", fmt.Sprintf(`{"assetId":%d}`, bid), 204)
	req(bob, "DELETE", fmt.Sprintf("%s/assets/%d", ap, aid), "", 403)
	req(carol, "GET", asset, "", 404)
	req(bob, "POST", asset+"/favorite", `{"favorite":true}`, 200)
	req(alice, "POST", asset+"/favorite", `{"favorite":false}`, 200)
	w = req(bob, "GET", asset, "", 200)
	if !strings.Contains(w.Body.String(), `"favorite":true`) {
		t.Fatal("favorites not private")
	}
	req(alice, "DELETE", ap+"/members/3", "", 204)
	req(bob, "GET", asset, "", 404)
	req(bob, "GET", asset+"/original", "", 404)
	req(alice, "POST", ap+"/members", `{"username":"bob","role":"viewer"}`, 204)
	req(admin, "DELETE", "/api/admin/users/2/folders/1", "", 204)
	req(bob, "GET", asset, "", 404)
	req(bob, "GET", asset+"/thumbnail/256", "", 404)
	for _, endpoint := range []string{"/api/scan", "/api/retry", "/api/admin/users/2/folders"} {
		req(bob, "POST", endpoint, `{}`, 403)
	}
	w = req(bob, "GET", "/api/stats", "", 200)
	var st struct{ Assets int }
	json.Unmarshal(w.Body.Bytes(), &st)
	if st.Assets != 1 {
		t.Fatal("statistics leaked", w.Body.String())
	}
	req(alice, "DELETE", ap, "", 204)
	req(admin, "GET", ap, "", 404)
	if _, e = os.Stat(filepath.Join(root, "Alice", "photo.jpg")); e != nil {
		t.Fatal("album deletion touched original")
	}
}
