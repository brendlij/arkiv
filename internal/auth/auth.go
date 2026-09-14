package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gallery/internal/access"
	"gallery/internal/config"
	"gallery/internal/media"
	"golang.org/x/crypto/argon2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

func Hash(password string) (string, error) {
	if len(password) < 12 || len(password) > 1024 {
		return "", fmt.Errorf("password must contain 12..1024 bytes")
	}
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}
func parseHash(hash string) ([]byte, []byte, error) {
	p := strings.Split(hash, "$")
	if len(p) != 6 || p[1] != "argon2id" || p[2] != "v=19" || p[3] != "m=65536,t=3,p=2" {
		return nil, nil, fmt.Errorf("unsupported hash; generate with arkiv hash-password")
	}
	s, e := base64.RawStdEncoding.DecodeString(p[4])
	if e != nil || len(s) != 16 {
		return nil, nil, fmt.Errorf("invalid salt")
	}
	k, e := base64.RawStdEncoding.DecodeString(p[5])
	if e != nil || len(k) != 32 {
		return nil, nil, fmt.Errorf("invalid hash")
	}
	return s, k, nil
}
func ValidateHash(h string) error { _, _, e := parseHash(h); return e }
func Verify(hash, password string) bool {
	s, k, e := parseHash(hash)
	if e != nil || len(password) > 1024 {
		return false
	}
	got := argon2.IDKey([]byte(password), s, 3, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(k, got) == 1
}

type attempt struct {
	count int
	until time.Time
}
type Auth struct {
	initErr  error
	db       *sql.DB
	c        config.Config
	mu       sync.Mutex
	attempts map[string]attempt
	hashSlot chan struct{}
}

func New(db *sql.DB, c config.Config) *Auth {
	return &Auth{initErr: Bootstrap(db, c), db: db, c: c, attempts: map[string]attempt{}, hashSlot: make(chan struct{}, 1)}
}
func (a *Auth) allowed(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for k, v := range a.attempts {
		if now.After(v.until) {
			delete(a.attempts, k)
		}
	}
	v := a.attempts[ip]
	if v.count >= 5 || len(a.attempts) >= 4096 {
		return false
	}
	if v.count == 0 {
		v.until = now.Add(5 * time.Minute)
	}
	v.count++
	a.attempts[ip] = v
	return true
}
func (a *Auth) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.initErr != nil {
			http.Error(w, "account initialization failed", 500)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: https://tile.openstreetmap.org; media-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		limit := int64(1 << 20)
		if r.URL.Path == "/api/uploads" && r.Method == "POST" {
			limit = media.VideoUploadLimit + (1 << 20)
		}
		if strings.HasPrefix(r.URL.Path, "/api/uploads/sessions/") && r.Method == "PATCH" {
			limit = 8 << 20
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		if strings.HasPrefix(r.URL.Path, "/share/") {
			w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
			w.Header().Set("Cache-Control", "no-store")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if r.Header.Get("X-Gallery-Request") != "1" {
				http.Error(w, "missing CSRF header", 403)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, e := url.Parse(origin)
				if e != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
					http.Error(w, "invalid origin", 403)
					return
				}
			}
		}
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/api/auth/login" && r.Method == "POST" {
			a.login(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/public/") && (r.Method == "GET" || r.Method == "HEAD") {
			w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
			next.ServeHTTP(w, r)
			return
		}
		u, ok := a.principal(r)
		if strings.HasPrefix(r.URL.Path, "/api/") && !ok {
			http.Error(w, "authentication required", 401)
			return
		}
		r = r.WithContext(access.WithUser(r.Context(), u))
		if r.URL.Path == "/api/auth/session" && r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"id": u.ID, "username": u.Username, "displayName": u.DisplayName, "role": u.Role, "mustChangePassword": u.MustChangePassword, "proxy": a.c.TrustProxy})
			return
		}
		if r.URL.Path == "/api/auth/password" && r.Method == "POST" {
			a.changePassword(w, r)
			return
		}
		if r.URL.Path == "/api/auth/logout" && r.Method == "POST" {
			if c, e := r.Cookie("gallery_session"); e == nil {
				if _, e = a.db.ExecContext(r.Context(), "DELETE FROM sessions WHERE token_hash=?", digest(c.Value)); e != nil {
					http.Error(w, "session removal failed", 500)
					return
				}
			}
			http.SetCookie(w, a.cookie("", -1))
			w.WriteHeader(204)
			return
		}
		if u.MustChangePassword && !a.c.TrustProxy && strings.HasPrefix(r.URL.Path, "/api/") {
			http.Error(w, "change your temporary password first", 403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/admin/users") && !strings.Contains(r.URL.Path, "/folders") && !strings.HasSuffix(r.URL.Path, "/storage") {
			a.manageUsers(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *Auth) principal(r *http.Request) (access.User, bool) {
	var u access.User
	if a.c.TrustProxy {
		host, _, e := net.SplitHostPort(r.RemoteAddr)
		if e != nil {
			return u, false
		}
		for _, n := range a.c.TrustedProxies {
			if n.Contains(net.ParseIP(host)) {
				u, _, e := Lookup(r.Context(), a.db, r.Header.Get("Remote-User"))
				return u, e == nil && u.Enabled
			}
		}
		return u, false
	}
	c, e := r.Cookie("gallery_session")
	if e != nil || len(c.Value) != 64 {
		return u, false
	}
	var hash, scope string
	e = a.db.QueryRowContext(r.Context(), "SELECT u.id,u.username,u.display_name,u.role,u.enabled,u.must_change_password,u.password_hash,s.credential_scope FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>? AND u.enabled=1", digest(c.Value), time.Now().Unix()).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Enabled, &u.MustChangePassword, &hash, &scope)
	return u, e == nil && scope == digest(u.Username+":"+hash)
}
func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	if a.c.TrustProxy {
		http.Error(w, "use trusted proxy login", 403)
		return
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if !a.allowed(host) {
		w.Header().Set("Retry-After", strconv.Itoa(300))
		http.Error(w, "too many attempts; retry in five minutes", 429)
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if e := json.NewDecoder(r.Body).Decode(&input); e != nil || len(input.Password) > 1024 {
		http.Error(w, "invalid request", 400)
		return
	}
	select {
	case a.hashSlot <- struct{}{}:
		defer func() { <-a.hashSlot }()
	default:
		http.Error(w, "login busy; retry shortly", 429)
		return
	}
	u, hash, lookupErr := Lookup(r.Context(), a.db, input.Username)
	if lookupErr != nil {
		hash = a.c.PasswordHash
	}
	valid := Verify(hash, input.Password)
	if !valid || lookupErr != nil || !u.Enabled {
		http.Error(w, "invalid credentials", 401)
		return
	}
	token := make([]byte, 32)
	if _, e := rand.Read(token); e != nil {
		http.Error(w, "session failure", 500)
		return
	}
	value := hex.EncodeToString(token)
	if _, e := a.db.ExecContext(r.Context(), "DELETE FROM sessions WHERE expires_at<=?", time.Now().Unix()); e != nil {
		http.Error(w, "session failure", 500)
		return
	}
	if _, e := a.db.ExecContext(r.Context(), "INSERT INTO sessions(token_hash,expires_at,credential_scope,user_id) VALUES(?,?,?,?)", digest(value), time.Now().Add(7*24*time.Hour).Unix(), digest(u.Username+":"+hash), u.ID); e != nil {
		http.Error(w, "session failure", 500)
		return
	}
	http.SetCookie(w, a.cookie(value, 7*24*3600))
	w.WriteHeader(204)
}
func (a *Auth) cookie(value string, age int) *http.Cookie {
	return &http.Cookie{Name: "gallery_session", Value: value, Path: "/", HttpOnly: true, Secure: a.c.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: age}
}
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
