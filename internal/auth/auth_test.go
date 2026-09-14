package auth

import (
	"gallery/internal/config"
	"gallery/internal/database"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionsAndCSRF(t *testing.T) {
	hash, e := Hash("correct-password-123")
	if e != nil {
		t.Fatal(e)
	}
	if !Verify(hash, "correct-password-123") || Verify(hash, "wrong") {
		t.Fatal("password verification")
	}
	db, e := database.Open(filepath.Join(t.TempDir(), "auth.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	a := New(db, config.Config{Username: "admin", PasswordHash: hash, SecureCookies: true})
	h := a.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	r := httptest.NewRequest("GET", "/api/assets", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("security headers missing")
	}
	tooLarge := httptest.NewRequest("POST", "http://gallery/api/auth/login", strings.NewReader(`{"username":"`+strings.Repeat("x", (1<<20)+1)+`"}`))
	tooLarge.Header.Set("X-Gallery-Request", "1")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, tooLarge)
	if w.Code != 400 {
		t.Fatal("oversized body accepted", w.Code)
	}
	login := func(csrf, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://gallery/api/auth/login", strings.NewReader(`{"username":"admin","password":"correct-password-123"}`))
		r.Header.Set("X-Gallery-Request", csrf)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w = login("", ""); w.Code != 403 {
		t.Fatal("no CSRF protection")
	}
	if w = login("1", "https://evil.example"); w.Code != 403 {
		t.Fatal("bad origin accepted")
	}
	w = login("1", "http://gallery")
	if w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("cookie flags")
	}
	r = httptest.NewRequest("GET", "/api/assets", nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	New(db, config.Config{Username: "admin", PasswordHash: hash}).Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("session failed after auth restart")
	}
	r = httptest.NewRequest("GET", "/api/assets", nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	db.Exec("UPDATE users SET username='renamed' WHERE id=1")
	New(db, config.Config{Username: "renamed", PasswordHash: hash}).Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("old session survived credential scope change")
	}
	for i := 0; i < 5; i++ {
		a.allowed("test")
	}
	if a.allowed("test") {
		t.Fatal("rate limit")
	}
}

func TestTrustedProxyBoundary(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "proxy.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, network, _ := net.ParseCIDR("127.0.0.1/32")
	a := New(db, config.Config{Username: "admin", TrustProxy: true, TrustedProxies: []*net.IPNet{network}})
	h := a.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	for _, test := range []struct {
		peer, user string
		want       int
	}{{"127.0.0.1:1234", "admin", 200}, {"192.0.2.1:1234", "admin", 401}, {"127.0.0.1:1234", "other", 401}, {"127.0.0.1:1234", "", 401}} {
		r := httptest.NewRequest("GET", "/api/assets", nil)
		r.RemoteAddr = test.peer
		r.Header.Set("Remote-User", test.user)
		r.Header.Set("X-Forwarded-For", "127.0.0.1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != test.want {
			t.Errorf("peer %s user %s: %d", test.peer, test.user, w.Code)
		}
	}
}

func TestChunkBodyLimit(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "chunks.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, network, _ := net.ParseCIDR("127.0.0.1/32")
	a := New(db, config.Config{Username: "admin", TrustProxy: true, TrustedProxies: []*net.IPNet{network}})
	h := a.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, e := io.Copy(io.Discard, r.Body); e != nil {
			http.Error(w, "too large", 413)
			return
		}
		w.WriteHeader(200)
	}))
	for _, tc := range []struct {
		method, path string
		size, want   int
	}{{"PATCH", "/api/uploads/sessions/test", 8 << 20, 200}, {"PATCH", "/api/uploads/sessions/test", (8 << 20) + 1, 413}, {"POST", "/api/albums", 2 << 20, 413}} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(strings.Repeat("x", tc.size)))
		r.RemoteAddr = "127.0.0.1:123"
		r.Header.Set("Remote-User", "admin")
		r.Header.Set("X-Gallery-Request", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
}
