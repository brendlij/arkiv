package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"gallery/internal/access"
	"gallery/internal/auth"
	"gallery/internal/config"
	"gallery/internal/database"
	"image"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResumableUploadRecoveryIsolationAndQuota(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, e := database.Open(dbPath)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { db.Close() }()
	if e = auth.Bootstrap(db, config.Config{Username: "admin"}); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{"INSERT INTO users(id,username,display_name,password_hash,role) VALUES(2,'alice','Alice','','member'),(3,'bob','Bob','','member')", "INSERT INTO libraries VALUES('arkiv-uploads','Uploads','unused',1)"} {
		if _, e = db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	var photo bytes.Buffer
	png.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	data := photo.Bytes()
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	s := &Server{DB: db, Uploads: root, UploadQuota: int64(len(data) * 2), UploadRetention: time.Hour}
	mux := http.NewServeMux()
	s.Register(mux)
	req := func(uid int64, method, path string, body []byte, offset int64, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Upload-Offset", fmt.Sprint(offset))
		role := "member"
		if uid == 1 {
			role = "admin"
		}
		r = r.WithContext(access.WithUser(r.Context(), access.User{ID: uid, Role: role}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w
	}
	decode := func(w *httptest.ResponseRecorder) uploadRecord {
		t.Helper()
		var v uploadRecord
		if e = json.Unmarshal(w.Body.Bytes(), &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	start := func(uid int64, key string, keep bool, hash string, want int) uploadRecord {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"filename": "sample.png", "size": len(data), "folder": "Trips/Summer", "sha256": hash, "clientKey": strings.Repeat(key, 20), "keepCopy": keep})
		w := req(uid, "POST", "/api/uploads/sessions", b, 0, want)
		if want != 200 {
			return uploadRecord{}
		}
		return decode(w)
	}
	v := start(2, "a", false, digest, 200)
	if v.State != "uploading" {
		t.Fatal(v.State)
	}
	if start(2, "a", false, digest, 200).ID != v.ID {
		t.Fatal("lost initiation response creates a second session")
	}
	req(0, "GET", "/api/uploads/sessions/"+v.ID, nil, 0, 401)
	req(3, "GET", "/api/uploads/sessions/"+v.ID, nil, 0, 404)
	req(3, "PATCH", "/api/uploads/sessions/"+v.ID, data[:20], 0, 404)
	req(2, "PATCH", "/api/uploads/sessions/"+v.ID, data[:20], 0, 200)
	req(2, "PATCH", "/api/uploads/sessions/"+v.ID, data[:20], 0, 409)
	// Simulate bytes reaching disk just before a crash but before acknowledgment.
	f, e := os.OpenFile(filepath.Join(root, ".incoming", v.ID+".part"), os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	f.Write([]byte("unacknowledged garbage"))
	f.Close()
	db.Close()
	db, e = database.Open(dbPath)
	if e != nil {
		t.Fatal(e)
	}
	s.DB = db
	status := decode(req(2, "GET", "/api/uploads/sessions/"+v.ID, nil, 0, 200))
	if status.Received != 20 {
		t.Fatal("acknowledged offset lost on restart")
	}
	req(2, "PATCH", "/api/uploads/sessions/"+v.ID, data[20:], 20, 200)
	done := decode(req(2, "POST", "/api/uploads/sessions/"+v.ID+"/complete", nil, 0, 200))
	if done.State != "done" || done.AssetID == 0 {
		t.Fatal(done)
	}
	again := decode(req(2, "POST", "/api/uploads/sessions/"+v.ID+"/complete", nil, 0, 200))
	if again.AssetID != done.AssetID {
		t.Fatal("duplicate completion")
	}
	var n int
	db.QueryRow("SELECT count(*) FROM assets").Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
	db.QueryRow("SELECT count(*) FROM jobs").Scan(&n)
	if n != 1 {
		t.Fatal("duplicate processing job")
	}
	db.Exec("UPDATE assets SET content_hash=NULL WHERE id=?", done.AssetID) // legacy uploads are fingerprinted on demand
	duplicate := start(2, "b", false, digest, 200)
	if duplicate.State != "duplicate" || duplicate.AssetID != done.AssetID {
		t.Fatal("duplicate not detected")
	}
	// Same hash in another account must reveal nothing about Alice's library.
	other := start(3, "c", false, digest, 200)
	if other.State != "uploading" || other.AssetID != 0 {
		t.Fatal("cross-user duplicate disclosure")
	}
	req(2, "DELETE", "/api/uploads/sessions/"+other.ID, nil, 0, 204) // nonexistent for Alice; no effect
	req(3, "GET", "/api/uploads/sessions/"+other.ID, nil, 0, 200)
	kept := start(2, "d", true, digest, 200)
	req(2, "POST", "/api/me/folders/manage", []byte(`{"action":"move","path":"Trips","target":"Moved"}`), 0, 409)
	start(2, "e", true, digest, 413)
	_, used, reserved, e := s.quota(context.Background(), 2)
	if e != nil || used != int64(len(data)) || reserved != int64(len(data)) {
		t.Fatalf("quota %d %d %v", used, reserved, e)
	}
	req(2, "DELETE", "/api/uploads/sessions/"+kept.ID, nil, 0, 204)
	// Administrators can configure an override; ordinary members cannot.
	req(2, "POST", "/api/admin/users/2/storage", []byte(`{"quotaBytes":10000}`), 0, 403)
	req(1, "POST", "/api/admin/users/2/storage", []byte(`{"quotaBytes":10000}`), 0, 200)
	bad := start(2, "f", true, digest, 200)
	broken := append([]byte{}, data...)
	broken[len(broken)-1] ^= 1
	req(2, "PATCH", "/api/uploads/sessions/"+bad.ID, broken, 0, 200)
	req(2, "POST", "/api/uploads/sessions/"+bad.ID+"/complete", nil, 0, 409)
	db.QueryRow("SELECT count(*) FROM assets").Scan(&n)
	if n != 1 {
		t.Fatal("corrupt bytes accepted")
	}
	req(2, "DELETE", "/api/uploads/sessions/"+bad.ID, nil, 0, 204)
	db.Exec("UPDATE assets SET trashed_at=1 WHERE id=?", done.AssetID)
	inTrash := start(2, "g", false, digest, 200)
	if !inTrash.Trashed || inTrash.State != "duplicate" {
		t.Fatal("trashed duplicate hidden or restored unexpectedly")
	}
	// Simulate a crash after final rename, before the asset transaction commits.
	recoverable := start(2, "h", true, digest, 200)
	req(2, "PATCH", "/api/uploads/sessions/"+recoverable.ID, data, 0, 200)
	db.Exec("UPDATE upload_sessions SET state='committing' WHERE id=?", recoverable.ID)
	final := filepath.Join(root, filepath.FromSlash(managedOriginalPath(2, recoverable.ID, ".png")))
	if e = os.MkdirAll(filepath.Dir(final), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(filepath.Join(root, ".incoming", recoverable.ID+".part"), final); e != nil {
		t.Fatal(e)
	}
	db.Close()
	db, e = database.Open(dbPath)
	if e != nil {
		t.Fatal(e)
	}
	s.DB = db
	if e = s.cleanUploads(context.Background()); e != nil {
		t.Fatal(e)
	}
	recovered := decode(req(2, "GET", "/api/uploads/sessions/"+recoverable.ID, nil, 0, 200))
	if recovered.State != "done" {
		t.Fatal("finalization not recovered")
	}
	original, e := os.ReadFile(final)
	if e != nil || !bytes.Equal(original, data) {
		t.Fatal("recovered original changed")
	}
}

func TestUploadCleanupPreservesOriginalsAndActiveTransfers(t *testing.T) {
	root := t.TempDir()
	db, e := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = auth.Bootstrap(db, config.Config{Username: "admin"}); e != nil {
		t.Fatal(e)
	}
	db.Exec("INSERT INTO libraries VALUES('arkiv-uploads','Uploads','unused',1)")
	s := &Server{DB: db, Uploads: root, UploadRetention: time.Hour}
	for _, dir := range []string{".incoming", "user-1"} {
		os.MkdirAll(filepath.Join(root, dir), 0700)
	}
	old := time.Now().Add(-2 * time.Hour)
	write := func(path string) {
		t.Helper()
		if e = os.WriteFile(filepath.Join(root, path), []byte("preserve bytes"), 0600); e != nil {
			t.Fatal(e)
		}
		os.Chtimes(filepath.Join(root, path), old, old)
	}
	active := strings.Repeat("a", 40)
	expired := strings.Repeat("b", 40)
	for _, v := range []struct {
		id      string
		updated int64
	}{{active, time.Now().Unix()}, {expired, old.Unix()}} {
		if _, e = db.Exec("INSERT INTO upload_sessions(id,owner_id,client_key,filename,folder,file_size,sha256,updated_at) VALUES(?,1,?,'x.png','user-1',100,?,?)", v.id, v.id, strings.Repeat("0", 64), v.updated); e != nil {
			t.Fatal(e)
		}
		write(".incoming/" + v.id + ".part")
	}
	write(".incoming/orphan.part")
	write("user-1/" + strings.Repeat("c", 40) + ".partial")
	orphan := "user-1/" + strings.Repeat("d", 40) + ".png"
	write(orphan)
	indexed := "user-1/" + strings.Repeat("e", 40) + ".png"
	write(indexed)
	if _, e = db.Exec("INSERT INTO assets(library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan) VALUES('arkiv-uploads',?,'user-1','saved.png','.png','image/png','image',14,1,'2026-01-01','upload')", indexed); e != nil {
		t.Fatal(e)
	}
	write("user-1/manual-photo.png")
	if e = s.cleanUploads(context.Background()); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{".incoming/" + active + ".part", indexed, "user-1/manual-photo.png", ".quarantine/" + orphan} {
		if _, e = os.Stat(filepath.Join(root, path)); e != nil {
			t.Fatalf("must preserve %s: %v", path, e)
		}
	}
	for _, path := range []string{".incoming/" + expired + ".part", ".incoming/orphan.part", "user-1/" + strings.Repeat("c", 40) + ".partial", orphan} {
		if _, e = os.Stat(filepath.Join(root, path)); !os.IsNotExist(e) {
			t.Fatalf("not cleaned %s", path)
		}
	}
	var n int
	db.QueryRow("SELECT count(*) FROM upload_quarantine").Scan(&n)
	if n != 1 {
		t.Fatal("missing quarantine record")
	}
	_, used, reserved, e := s.quota(context.Background(), 1)
	if e != nil || used != 28 || reserved != 100 {
		t.Fatalf("quarantine/reservation accounting %d %d %v", used, reserved, e)
	}
}

func TestStorageRouteThroughAuthentication(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "auth-storage.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, network, _ := net.ParseCIDR("127.0.0.1/32")
	a := auth.New(db, config.Config{Username: "admin", TrustProxy: true, TrustedProxies: []*net.IPNet{network}})
	mux := http.NewServeMux()
	(&Server{DB: db}).Register(mux)
	r := httptest.NewRequest("GET", "/api/admin/users/1/storage", nil)
	r.RemoteAddr = "127.0.0.1:123"
	r.Header.Set("Remote-User", "admin")
	w := httptest.NewRecorder()
	a.Handler(mux).ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "quota") {
		t.Fatalf("quota route intercepted: %d %s", w.Code, w.Body.String())
	}
}
