package api

import (
	"encoding/json"
	"gallery/internal/auth"
	"gallery/internal/config"
	"gallery/internal/database"
	"gallery/internal/thumbnails"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublicAndTimedSharing(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "share.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	root, cache := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(root, "one.jpg"), []byte("original"), 0600)
	_, e = db.Exec("INSERT INTO libraries VALUES('photos','Photos',?,1)", root)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`INSERT INTO assets(library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan,preview_status) VALUES('photos','one.jpg','','one.jpg','.jpg','image/jpeg','image',8,1,'2026','test','ready'),('photos','other.jpg','','other.jpg','.jpg','image/jpeg','image',8,1,'2026','test','ready');`)
	if e != nil {
		t.Fatal(e)
	}
	key := strings.Repeat("b", 64)
	db.Exec("UPDATE assets SET preview_key=?", key)
	p := thumbnails.Path(cache, key, 256)
	os.MkdirAll(filepath.Dir(p), 0700)
	os.WriteFile(p, []byte("preview"), 0600)
	_, network, _ := net.ParseCIDR("127.0.0.1/32")
	a := auth.New(db, config.Config{Username: "admin", TrustProxy: true, TrustedProxies: []*net.IPNet{network}})
	if a.InitError() != nil {
		t.Fatal(a.InitError())
	}
	db.Exec("INSERT INTO users(username,display_name,password_hash,role) VALUES('member','Member','','member')")
	db.Exec("INSERT INTO albums(name,owner_id) VALUES('Shared story',1),('Private story',1)")
	db.Exec("INSERT INTO album_assets(album_id,asset_id,added_by) VALUES(1,1,1),(2,2,1)")
	mux := http.NewServeMux()
	(&Server{DB: db, Cache: cache}).Register(mux)
	h := a.Handler(mux)
	req := func(user, method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1000"
		r.Header.Set("X-Gallery-Request", "1")
		r.Header.Set("Remote-User", user)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s %s = %d want %d: %s", user, method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	create := func(body string) string {
		t.Helper()
		w := req("admin", "POST", "/api/albums/1/public-link", body, 200)
		var result struct{ Path string }
		if e = json.Unmarshal(w.Body.Bytes(), &result); e != nil {
			t.Fatal(e)
		}
		return strings.TrimPrefix(result.Path, "/share/")
	}
	token := create(`{}`)
	prefix := "/api/public/" + token
	w := req("", "GET", prefix, "", 200)
	if !strings.Contains(w.Body.String(), "one.jpg") || strings.Contains(w.Body.String(), "other.jpg") || strings.Contains(w.Body.String(), "relativePath") || strings.Contains(w.Body.String(), "latitude") {
		t.Fatal("public listing scope or metadata", w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("unsafe cache/referrer policy")
	}
	req("", "GET", prefix+"/assets/1/thumbnail/256", "", 200)
	req("", "HEAD", prefix+"/assets/1/thumbnail/256", "", 200)
	req("", "GET", prefix+"/assets/2/thumbnail/256", "", 404)
	req("", "GET", prefix+"/assets/1/original?download=1", "", 403)
	req("", "GET", "/api/assets/1", "", 401)
	req("", "GET", "/api/albums/1", "", 401)
	req("", "POST", prefix, `{}`, 401)
	req("member", "POST", "/api/albums/1/public-link", `{}`, 403)
	req("", "GET", "/api/public/invalid", "", 404)
	var stored string
	db.QueryRow("SELECT token_hash FROM album_public_shares WHERE album_id=1").Scan(&stored)
	if stored == token || len(stored) != 64 {
		t.Fatal("token was not hashed")
	}
	settings := req("admin", "GET", "/api/albums/1/public-link", "", 200)
	if strings.Contains(settings.Body.String(), token) || strings.Contains(settings.Body.String(), stored) {
		t.Fatal("settings expose secret")
	}
	old := prefix
	token = create(`{"allowDownloads":true}`)
	prefix = "/api/public/" + token
	req("", "GET", old, "", 404)
	w = req("", "GET", prefix+"/assets/1/original", "", 200)
	if w.Body.String() != "original" {
		t.Fatal("download missing")
	}
	req("", "GET", prefix+"/assets/2/original", "", 404)
	db.Exec("UPDATE album_public_shares SET expires_at=unixepoch()")
	req("", "GET", prefix, "", 404)
	req("", "GET", prefix+"/assets/1/original", "", 404)
	req("", "GET", prefix+"/assets/1/thumbnail/256", "", 404)
	db.Exec("UPDATE album_public_shares SET expires_at=0,starts_at=unixepoch()+100")
	req("", "GET", prefix, "", 404)
	db.Exec("UPDATE album_public_shares SET starts_at=unixepoch()-1")
	req("", "GET", prefix, "", 200)
	db.Exec("UPDATE users SET enabled=0 WHERE id=1")
	req("", "GET", prefix, "", 404)
	db.Exec("UPDATE users SET enabled=1 WHERE id=1")
	req("admin", "DELETE", "/api/albums/1/public-link", "", 204)
	req("", "GET", prefix, "", 404)
	for _, body := range []string{`{"expiresAt":"1970-01-01T00:00:00Z"}`, `{"expiresAt":"yesterday"}`, `{"expiresAt":"2000-01-01T00:00:00Z"}`, `{"startsAt":"2099-01-02T00:00:00Z","expiresAt":"2099-01-01T00:00:00Z"}`} {
		req("admin", "POST", "/api/albums/1/public-link", body, 400)
	}
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	body, _ := json.Marshal(map[string]string{"username": "member", "role": "contributor", "startsAt": future})
	req("admin", "POST", "/api/albums/1/members", string(body), 204)
	req("member", "GET", "/api/albums/1", "", 404)
	req("member", "GET", "/api/assets/1", "", 404)
	req("member", "POST", "/api/albums/1/assets", `{"assetId":1}`, 404)
	db.Exec("UPDATE album_members SET starts_at=unixepoch()-1,expires_at=unixepoch()+100")
	req("member", "GET", "/api/albums/1", "", 200)
	req("member", "GET", "/api/assets/1/original", "", 200)
	db.Exec("UPDATE album_members SET expires_at=unixepoch()")
	req("member", "GET", "/api/albums/1", "", 404)
	req("member", "GET", "/api/assets/1", "", 404)
	req("member", "GET", "/api/assets/1/thumbnail/256", "", 404)
	req("member", "DELETE", "/api/albums/1/assets/1", "", 404)
	w = req("member", "GET", "/api/albums?scope=shared", "", 200)
	if strings.Contains(w.Body.String(), "Shared story") {
		t.Fatal("expired album listed")
	}
	// A public link must also stop serving a contributor's photos after source access is revoked.
	db.Exec("INSERT INTO folder_grants(user_id,library_id,folder,label) VALUES(2,'photos','','Photos')")
	db.Exec("UPDATE album_assets SET added_by=2 WHERE album_id=1")
	token = create(`{}`)
	prefix = "/api/public/" + token
	req("", "GET", prefix+"/assets/1/thumbnail/256", "", 200)
	db.Exec("DELETE FROM folder_grants WHERE user_id=2")
	req("", "GET", prefix+"/assets/1/thumbnail/256", "", 404)
}
