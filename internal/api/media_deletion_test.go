package api

import (
	"bytes"
	"context"
	"fmt"
	"gallery/internal/access"
	"gallery/internal/auth"
	"gallery/internal/config"
	"gallery/internal/database"
	"gallery/internal/scanner"
	"gallery/internal/thumbnails"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersonalRemovalAndPermanentUploadDeletion(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "gallery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = auth.Bootstrap(db, config.Config{Username: "admin"}); err != nil {
		t.Fatal(err)
	}
	uploads, external, cache := t.TempDir(), t.TempDir(), t.TempDir()
	for _, path := range []string{"user-2", "user-3"} {
		if err = os.MkdirAll(filepath.Join(uploads, path), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{filepath.Join(uploads, "user-2/a.jpg"), filepath.Join(uploads, "user-3/b.jpg"), filepath.Join(external, "external.jpg")} {
		if err = os.WriteFile(path, []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		"INSERT INTO users(id,username,display_name,password_hash,role) VALUES(2,'alice','Alice','','member'),(3,'bob','Bob','','member')",
		"INSERT INTO libraries VALUES('arkiv-uploads','Uploads','unused',1),('external','External','unused2',1)",
		"INSERT INTO folder_grants(user_id,library_id,folder,label) VALUES(2,'arkiv-uploads','','Uploads'),(3,'arkiv-uploads','','Uploads'),(2,'external','','External')",
		"INSERT INTO assets(id,library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan) VALUES(1,'arkiv-uploads','user-2/a.jpg','user-2','a.jpg','.jpg','image/jpeg','image',8,1,'2026-01-01','upload'),(2,'arkiv-uploads','user-3/b.jpg','user-3','b.jpg','.jpg','image/jpeg','image',8,1,'2026-01-01','upload')",
		"INSERT INTO albums(id,name,owner_id) VALUES(1,'Shared',2)",
		"INSERT INTO album_assets(album_id,asset_id,added_by) VALUES(1,1,2)",
		"INSERT INTO user_favorites(user_id,asset_id) VALUES(2,1)",
		"INSERT INTO jobs(asset_id) VALUES(1)",
		"INSERT INTO upload_sessions(id,owner_id,client_key,filename,folder,file_size,sha256,received,state,asset_id,updated_at) VALUES('done',2,'key','a.jpg','user-2',8,'hash',8,'done',1,1)",
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err, q)
		}
	}
	if _, err = db.Exec("UPDATE libraries SET root=? WHERE id='arkiv-uploads'", uploads); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE libraries SET root=? WHERE id='external'", external); err != nil {
		t.Fatal(err)
	}
	sc := &scanner.Scanner{DB: db, Libraries: []config.Library{{ID: "external", Name: "External", Root: external, Enabled: true}}}
	if _, err = sc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := &Server{DB: db, Uploads: uploads, Cache: cache}
	mux := http.NewServeMux()
	s.Register(mux)
	user := access.User{ID: 2, Role: "member"}
	request := func(method, url, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, url, bytes.NewBufferString(body))
		r = r.WithContext(access.WithUser(r.Context(), user))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, url, w.Code, w.Body.String())
		}
		return w
	}
	visible := func(u access.User, id int64, filter string, want int) {
		t.Helper()
		var n int
		condition := access.Visible(u, "a")
		if filter == "trash" {
			condition = access.Trash(u, "a")
		}
		if err = db.QueryRow("SELECT count(*) FROM assets a WHERE a.id=? AND ("+condition+")", id).Scan(&n); err != nil || n != want {
			t.Fatalf("visibility %s id %d: %d %v", filter, id, n, err)
		}
	}
	// Borrowed uploads can be removed personally, never physically deleted.
	request("POST", "/api/assets/bulk", `{"ids":[2],"action":"trash"}`, 200)
	visible(user, 2, "photos", 0)
	visible(user, 2, "trash", 1)
	visible(access.User{ID: 3, Role: "member"}, 2, "photos", 1)
	request("GET", "/api/assets/2/original", "", 200)
	request("POST", "/api/assets/bulk", `{"ids":[2],"action":"restore"}`, 200)
	visible(user, 2, "photos", 1)
	request("POST", "/api/assets/bulk", `{"ids":[2],"action":"delete","confirm":true}`, 403)
	request("POST", "/api/assets/bulk", `{"ids":[2],"action":"trash"}`, 200)
	request("POST", "/api/assets/bulk", `{"ids":[2],"action":"delete"}`, 400)
	request("POST", "/api/assets/bulk", `{"ids":[2],"action":"delete","confirm":true}`, 200)
	visible(user, 2, "trash", 0)
	visible(user, 2, "photos", 0)
	visible(access.User{ID: 3, Role: "member"}, 2, "photos", 1)
	if _, err = os.Stat(filepath.Join(uploads, "user-3/b.jpg")); err != nil {
		t.Fatal("borrowed original removed", err)
	}
	// Read-only external photos stay hidden through an unchanged rescan.
	request("POST", "/api/assets/bulk", `{"ids":[3],"action":"trash"}`, 200)
	request("POST", "/api/assets/bulk", `{"ids":[3],"action":"delete","confirm":true}`, 200)
	if _, err = sc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	visible(user, 3, "photos", 0)
	if _, err = os.Stat(filepath.Join(external, "external.jpg")); err != nil {
		t.Fatal("external original removed", err)
	}
	// A revoked grant does not keep a borrowed photo accessible through Trash.
	if _, err = db.Exec("DELETE FROM personal_trash WHERE user_id=2 AND asset_id=2"); err != nil {
		t.Fatal(err)
	}
	request("POST", "/api/assets/bulk", `{"ids":[2],"action":"trash"}`, 200)
	if _, err = db.Exec("DELETE FROM folder_grants WHERE user_id=2 AND library_id='arkiv-uploads'"); err != nil {
		t.Fatal(err)
	}
	visible(user, 2, "trash", 0)
	request("GET", "/api/assets/2/original", "", 404)
	// Owned originals, derivatives, receipts, favorites and album links are removed.
	key := strings.Repeat("a", 64)
	if _, err = db.Exec("UPDATE assets SET preview_key=? WHERE id=1", key); err != nil {
		t.Fatal(err)
	}
	for _, size := range thumbnails.Sizes {
		path := thumbnails.Path(cache, key, size)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, []byte("preview"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec("INSERT INTO folder_grants(user_id,library_id,folder,label) VALUES(2,'arkiv-uploads','user-2','Uploads')"); err != nil {
		t.Fatal(err)
	}
	request("POST", "/api/assets/bulk", `{"ids":[1],"action":"trash"}`, 200)
	// Simulate temporarily unavailable upload mount: intent must persist safely.
	s.Uploads = filepath.Join(t.TempDir(), "unavailable")
	request("POST", "/api/assets/bulk", `{"ids":[1,99],"action":"delete","confirm":true}`, 403)
	request("POST", "/api/assets/bulk", `{"ids":[1],"action":"delete","confirm":true}`, 200)
	visible(user, 1, "trash", 0)
	request("GET", "/api/assets/1/original", "", 404)
	var n int
	if err = db.QueryRow("SELECT count(*) FROM media_deletions").Scan(&n); err != nil || n != 1 {
		t.Fatal("deletion intent lost", n, err)
	}
	s.Uploads = uploads
	if err = s.cleanUploads(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(uploads, "user-2/a.jpg")); !os.IsNotExist(err) {
		t.Fatal("owned original retained", err)
	}
	for _, size := range thumbnails.Sizes {
		if _, err = os.Stat(thumbnails.Path(cache, key, size)); !os.IsNotExist(err) {
			t.Fatal("derivative retained", err)
		}
	}
	for _, table := range []string{"assets", "jobs", "user_favorites", "album_assets", "upload_sessions", "media_deletions"} {
		col := "asset_id"
		if table == "assets" {
			col = "id"
		}
		if err = db.QueryRow(fmt.Sprintf("SELECT count(*) FROM %s WHERE %s=1", table, col)).Scan(&n); err != nil || n != 0 {
			t.Fatal(table, n, err)
		}
	}
	if err = s.cleanUploads(context.Background()); err != nil {
		t.Fatal("cleanup not idempotent", err)
	}
}
