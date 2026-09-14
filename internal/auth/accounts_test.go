package auth

import (
	"fmt"
	"gallery/internal/config"
	"gallery/internal/database"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountLifecycle(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "auth.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	hash, _ := Hash("admin-password-123")
	a := New(db, config.Config{Username: "admin", PasswordHash: hash})
	h := a.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	attempt := 0
	req := func(cookie *http.Cookie, method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		attempt++
		r.RemoteAddr = fmt.Sprintf("192.0.2.%d:1000", attempt)
		r.Header.Set("X-Gallery-Request", "1")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	admin := req(nil, "POST", "/api/auth/login", `{"username":"admin","password":"admin-password-123"}`, 204).Result().Cookies()[0]
	req(admin, "POST", "/api/admin/users", `{"username":"alice","displayName":"Alice","role":"member","password":"temporary-password-123"}`, 201)
	alice := req(nil, "POST", "/api/auth/login", `{"username":"alice","password":"temporary-password-123"}`, 204).Result().Cookies()[0]
	req(alice, "GET", "/api/assets", "", 403)
	w := req(alice, "GET", "/api/auth/session", "", 200)
	if !strings.Contains(w.Body.String(), `"mustChangePassword":true`) {
		t.Fatal("missing onboarding flag")
	}
	req(alice, "POST", "/api/auth/password", `{"currentPassword":"temporary-password-123","newPassword":"personal-password-123"}`, 204)
	req(alice, "GET", "/api/auth/session", "", 401)
	alice = req(nil, "POST", "/api/auth/login", `{"username":"alice","password":"personal-password-123"}`, 204).Result().Cookies()[0]
	req(alice, "GET", "/api/assets", "", 200)
	req(alice, "GET", "/api/admin/users", "", 403)
	req(admin, "PATCH", "/api/admin/users/1", `{"displayName":"Admin","role":"member","enabled":true}`, 409)
	req(admin, "POST", "/api/admin/users/2/password", `{"password":"reset-password-123"}`, 204)
	req(alice, "GET", "/api/assets", "", 401)
	alice = req(nil, "POST", "/api/auth/login", `{"username":"alice","password":"reset-password-123"}`, 204).Result().Cookies()[0]
	req(alice, "GET", "/api/assets", "", 403)
	req(admin, "PATCH", "/api/admin/users/2", `{"displayName":"Alice","role":"member","enabled":false}`, 204)
	req(alice, "GET", "/api/auth/session", "", 401)
	req(nil, "POST", "/api/auth/login", `{"username":"alice","password":"reset-password-123"}`, 401)
	req(admin, "POST", "/api/auth/password", `{"currentPassword":"admin-password-123","newPassword":"new-admin-password-123"}`, 204)
	if e = Bootstrap(db, config.Config{Username: "replacement", PasswordHash: hash}); e != nil {
		t.Fatal(e)
	}
	var stored string
	db.QueryRow("SELECT password_hash FROM users WHERE id=1").Scan(&stored)
	if !Verify(stored, "new-admin-password-123") {
		t.Fatal("restart undid changed password")
	}
	var n int
	db.QueryRow("SELECT count(*) FROM users").Scan(&n)
	if n != 2 {
		t.Fatal("restart duplicated bootstrap")
	}
}
func TestBootstrapPreservesLegacyCollections(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "upgrade.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, e = db.Exec(`INSERT INTO libraries VALUES('photos','Photos','/photos',1); INSERT INTO assets(library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan,favorite) VALUES('photos','a.jpg','','a.jpg','.jpg','image/jpeg','image',1,1,'2026','old',1); INSERT INTO albums(name) VALUES('Legacy album'); INSERT INTO album_assets(album_id,asset_id) VALUES(1,1);`)
	if e != nil {
		t.Fatal(e)
	}
	if e = Bootstrap(db, config.Config{Username: "admin"}); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{"SELECT count(*) FROM albums WHERE owner_id=1", "SELECT count(*) FROM album_assets WHERE added_by=1", "SELECT count(*) FROM user_favorites WHERE user_id=1 AND asset_id=1"} {
		var n int
		if e = db.QueryRow(q).Scan(&n); e != nil || n != 1 {
			t.Fatal(q, n, e)
		}
	}
}
