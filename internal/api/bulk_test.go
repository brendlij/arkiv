package api

import (
	"bytes"
	"encoding/json"
	"gallery/internal/access"
	"gallery/internal/auth"
	"gallery/internal/config"
	"gallery/internal/database"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestBulkOwnershipAtomicityAndTrash(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = auth.Bootstrap(db, config.Config{Username: "admin"}); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{
		"INSERT INTO users(id,username,display_name,password_hash,role) VALUES(2,'alice','Alice','','member'),(3,'bob','Bob','','member')",
		"INSERT INTO libraries VALUES('arkiv-uploads','Uploads','unused',1),('external','External','unused2',1)",
		"INSERT INTO folder_grants(user_id,library_id,folder,label) VALUES(2,'arkiv-uploads','user-2','Uploads'),(3,'arkiv-uploads','user-2','Shared')",
		"INSERT INTO assets(id,library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan) VALUES(1,'arkiv-uploads','user-2/a.jpg','user-2','a.jpg','.jpg','image/jpeg','image',10,1,'2026-01-01','upload'),(2,'arkiv-uploads','user-3/b.jpg','user-3','b.jpg','.jpg','image/jpeg','image',20,1,'2026-01-01','upload'),(3,'external','a.jpg','','a.jpg','.jpg','image/jpeg','image',30,1,'2026-01-01','scan')",
		"INSERT INTO albums(id,name,owner_id) VALUES(1,'Shared',2)",
		"INSERT INTO album_assets(album_id,asset_id,added_by) VALUES(1,1,2)",
	} {
		if _, e = db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	mux := http.NewServeMux()
	(&Server{DB: db}).Register(mux)
	request := func(body string, want int) {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/assets/bulk", bytes.NewBufferString(body))
		r = r.WithContext(access.WithUser(r.Context(), access.User{ID: 2, Role: "member"}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	// Access explanations must not disclose the audience to a borrowed viewer.
	accessInfo := func(uid int64, url string, want int) map[string]any {
		t.Helper()
		r := httptest.NewRequest("GET", url, nil)
		r = r.WithContext(access.WithUser(r.Context(), access.User{ID: uid, Role: "member"}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("access details: %d %s", w.Code, w.Body.String())
		}
		result := map[string]any{}
		if want == 200 {
			if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil {
				t.Fatal(e)
			}
		}
		return result
	}
	ownerInfo := accessInfo(2, "/api/access-details?asset=1", 200)
	if ownerInfo["owner"] != "Alice (you)" || ownerInfo["detailed"] != true || len(ownerInfo["people"].([]any)) != 3 {
		t.Fatalf("owner access: %+v", ownerInfo)
	}
	borrowed := accessInfo(3, "/api/access-details?asset=1", 200)
	if borrowed["detailed"] != false || len(borrowed["people"].([]any)) != 0 {
		t.Fatal("borrowed viewer can enumerate people")
	}
	accessInfo(2, "/api/access-details?asset=2", 404)
	accessInfo(0, "/api/access-details?asset=1", 401)
	folderInfo := accessInfo(2, "/api/access-details?library=arkiv-uploads&folder=user-2", 200)
	if folderInfo["canManage"] != true {
		t.Fatal("uploaded folder ownership missing")
	}
	if _, e := db.Exec("INSERT INTO album_public_shares VALUES(1,'test-hash',0,0,0,1)"); e != nil {
		t.Fatal(e)
	}
	publicInfo := accessInfo(2, "/api/access-details?asset=1", 200)
	if len(publicInfo["publicAlbums"].([]any)) != 1 {
		t.Fatal("active public share missing")
	}
	request(`{"ids":[1,2],"action":"trash"}`, 403)
	request(`{"ids":[1,3],"action":"trash"}`, 403)
	request(`{"ids":[1],"action":"move","folder":"../escape"}`, 400)
	var folder, rel string
	var deleted int
	check := func(wantFolder string, wantDeleted int) {
		t.Helper()
		if e := db.QueryRow("SELECT folder,relative_path,trashed_at FROM assets WHERE id=1").Scan(&folder, &rel, &deleted); e != nil {
			t.Fatal(e)
		}
		if folder != wantFolder || rel != "user-2/a.jpg" || (deleted > 0) != (wantDeleted > 0) {
			t.Fatalf("%s %s %d", folder, rel, deleted)
		}
	}
	check("user-2", 0)
	request(`{"ids":[1],"action":"move","folder":"Holidays/Summer"}`, 200)
	check("user-2/Holidays/Summer", 0)
	request(`{"ids":[1],"action":"trash"}`, 200)
	check("user-2/Holidays/Summer", 1)
	trashInfo := accessInfo(2, "/api/access-details?asset=1", 200)
	if len(trashInfo["people"].([]any)) != 1 || len(trashInfo["publicAlbums"].([]any)) != 0 {
		t.Fatal("trash audience incorrect")
	}
	accessInfo(3, "/api/access-details?asset=1", 404)
	for _, u := range []access.User{{ID: 1, Role: "admin"}, {ID: 2, Role: "member"}, {ID: 3, Role: "member"}, {PublicAlbum: 1}} {
		var n int
		if e := db.QueryRow("SELECT count(*) FROM assets a WHERE a.id=1 AND " + access.Visible(u, "a")).Scan(&n); e != nil || n != 0 {
			t.Fatalf("trash visible to %+v: %d %v", u, n, e)
		}
	}
	request(`{"ids":[1],"action":"move","folder":"Other"}`, 409)
	request(`{"ids":[1],"action":"restore"}`, 200)
	check("user-2/Holidays/Summer", 0)
	var n int
	db.QueryRow("SELECT count(*) FROM assets a WHERE a.id=1 AND " + access.Visible(access.User{PublicAlbum: 1}, "a")).Scan(&n)
	if n != 1 {
		t.Fatal("restored album item unavailable")
	}
	// Persist empty folders and keep recursive shares attached to renamed trees.
	folderAction := func(uid int64, body string, want int) {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/me/folders/manage", bytes.NewBufferString(body))
		r = r.WithContext(access.WithUser(r.Context(), access.User{ID: uid, DisplayName: "Alice", Role: "member"}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("folder %s: %d %s", body, w.Code, w.Body.String())
		}
	}
	folderAction(2, `{"action":"create","path":"Holidays/Summer/Empty"}`, 200)
	folderAction(2, `{"action":"create","path":"Holidays/Summer/Empty"}`, 409)
	folderAction(2, `{"action":"create","path":"../escape"}`, 400)
	folderAction(0, `{"action":"create","path":"x"}`, 401)
	folderAction(3, `{"action":"move","path":"Holidays","target":"Stolen"}`, 404)
	// Remove the fixture's broad grant to isolate the new share.
	if _, e := db.Exec("DELETE FROM folder_grants WHERE user_id=3"); e != nil {
		t.Fatal(e)
	}
	folderAction(2, `{"action":"share","path":"Holidays/Summer","username":"bob"}`, 200)
	accessInfo(3, "/api/access-details?library=arkiv-uploads&folder=user-2/Holidays/Summer/Empty", 200)
	request(`{"ids":[1],"action":"trash"}`, 200)
	folderAction(2, `{"action":"move","path":"Holidays","target":"Trips"}`, 200)
	check("user-2/Trips/Summer", 1)
	var emptyCount, grantCount int
	db.QueryRow("SELECT count(*) FROM managed_folders WHERE path='user-2/Trips/Summer/Empty'").Scan(&emptyCount)
	db.QueryRow("SELECT count(*) FROM folder_grants WHERE user_id=3 AND folder='user-2/Trips/Summer'").Scan(&grantCount)
	if emptyCount != 1 || grantCount != 1 {
		t.Fatal("empty subtree or grant lost on move")
	}
	folderAction(2, `{"action":"move","path":"Trips","target":"Trips/Nested"}`, 400)
	folderAction(2, `{"action":"move","path":"","target":"Other"}`, 400)
	folderAction(2, `{"action":"create","path":"Existing"}`, 200)
	folderAction(2, `{"action":"move","path":"Trips","target":"Existing"}`, 409)
	folderAction(2, `{"action":"revoke","path":"Trips/Summer","username":"bob"}`, 200)
	accessInfo(3, "/api/access-details?library=arkiv-uploads&folder=user-2/Trips/Summer/Empty", 404)
	request(`{"ids":[1],"action":"restore"}`, 200)
	check("user-2/Trips/Summer", 0)

	adminGrant := httptest.NewRequest("POST", "/api/admin/users/3/folders", bytes.NewBufferString(`{"libraryId":"arkiv-uploads","folder":"user-2/Trips/Summer/Empty","label":"Empty shared folder"}`))
	adminGrant = adminGrant.WithContext(access.WithUser(adminGrant.Context(), access.User{ID: 1, Role: "admin"}))
	grantResponse := httptest.NewRecorder()
	mux.ServeHTTP(grantResponse, adminGrant)
	if grantResponse.Code != 204 {
		t.Fatalf("admin cannot grant virtual folder: %d %s", grantResponse.Code, grantResponse.Body.String())
	}
	accessInfo(3, "/api/access-details?library=arkiv-uploads&folder=user-2/Trips/Summer/Empty", 200)

}
