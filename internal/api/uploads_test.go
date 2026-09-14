package api

import (
	"bytes"
	"encoding/binary"
	"gallery/internal/access"
	"gallery/internal/auth"
	"gallery/internal/config"
	"gallery/internal/database"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadIsolationAndValidation(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = auth.Bootstrap(db, config.Config{Username: "admin"}); e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	db.Exec("INSERT INTO libraries VALUES('arkiv-uploads','Uploads',?,1)", root)
	db.Exec("INSERT INTO users(username,display_name,password_hash,role) VALUES('alice','Alice','','member'),('bob','Bob','','member')")
	mux := http.NewServeMux()
	(&Server{DB: db, Uploads: root}).Register(mux)
	var photo bytes.Buffer
	png.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	upload := func(uid int64, name string, data []byte, want int) {
		t.Helper()
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		p, _ := form.CreateFormFile("file", name)
		p.Write(data)
		form.Close()
		r := httptest.NewRequest("POST", "/api/uploads?folder=Holidays%2FSummer", &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		r = r.WithContext(access.WithUser(r.Context(), access.User{ID: uid, Role: "member"}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	upload(0, "test.png", photo.Bytes(), 401)
	upload(2, "test.png", photo.Bytes(), 201)
	upload(2, "test.png", photo.Bytes(), 201)
	upload(2, "fake.png", []byte("not an image"), 415)
	upload(2, "fake.jpg", photo.Bytes(), 415)
	upload(2, "script.svg", []byte("<svg/>"), 415)
	upload(2, "bad\\file.png", photo.Bytes(), 400)
	video := make([]byte, 24)
	binary.BigEndian.PutUint32(video, 24)
	copy(video[4:], "ftyp")
	copy(video[8:], "isom")
	copy(video[16:], "isom")
	upload(2, "clip.MP4", video, 201)
	upload(2, "camera.dng", append([]byte("II*\x00"), make([]byte, 16)...), 201)
	upload(2, "fake.mp4", photo.Bytes(), 415)
	var kind, mime string
	if e := db.QueryRow("SELECT media_type,mime_type FROM assets WHERE filename='clip.MP4'").Scan(&kind, &mime); e != nil || kind != "video" || mime != "video/mp4" {
		t.Fatalf("video classification: %s %s %v", kind, mime, e)
	}
	if e := db.QueryRow("SELECT media_type FROM assets WHERE filename='camera.dng'").Scan(&kind); e != nil || kind != "raw" {
		t.Fatalf("RAW classification: %s %v", kind, e)
	}
	var count int
	db.QueryRow("SELECT count(*) FROM assets").Scan(&count)
	if count != 4 {
		t.Fatal(count)
	}
	db.QueryRow("SELECT count(*) FROM jobs").Scan(&count)
	if count != 4 {
		t.Fatal(count)
	}
	db.QueryRow("SELECT count(*) FROM assets a WHERE " + access.Direct(access.User{ID: 3, Role: "member"}, "a")).Scan(&count)
	if count != 0 {
		t.Fatal("another member can see uploads")
	}
	db.QueryRow("SELECT count(*) FROM assets a WHERE " + access.Direct(access.User{ID: 2, Role: "member"}, "a")).Scan(&count)
	if count != 4 {
		t.Fatal("owner cannot see uploads")
	}
	var savedFolder, storedPath string
	if e := db.QueryRow("SELECT folder,relative_path FROM assets LIMIT 1").Scan(&savedFolder, &storedPath); e != nil {
		t.Fatal(e)
	}
	if savedFolder != "user-2/Holidays/Summer" || !strings.HasPrefix(storedPath, "user-2/") || strings.Contains(storedPath, "Holidays") {
		t.Fatal("upload destination altered immutable storage or was ignored")
	}
	db.QueryRow("SELECT count(*) FROM managed_folders WHERE owner_id=2").Scan(&count)
	if count != 3 {
		t.Fatalf("missing upload parents: %d", count)
	}
	db.Exec("UPDATE assets SET file_size=?", uploadQuota)
	upload(2, "full.png", photo.Bytes(), 413)
}
