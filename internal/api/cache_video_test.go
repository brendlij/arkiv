package api

import (
	"context"
	"crypto/sha256"
	"gallery/internal/access"
	"gallery/internal/database"
	"gallery/internal/thumbnails"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCacheMaintenanceProtectsOriginalsAndRepairs(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	key := thumbnails.Key(1, 1, 10, 1)
	for _, q := range []string{"INSERT INTO libraries VALUES('photos','Photos','unused',1)", "INSERT INTO assets(id,library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan,preview_key,preview_status) VALUES(1,'photos','a.jpg','','a.jpg','.jpg','image/jpeg','image',10,1,'2026-01-01','scan','" + key + "','ready')"} {
		if _, e = db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	cache := t.TempDir()
	s := Server{DB: db, Cache: cache}
	old := time.Now().Add(-48 * time.Hour)
	orphan := thumbnails.Path(cache, thumbnails.Key(2, 1, 10, 1), 256)
	fresh := thumbnails.Path(cache, thumbnails.Key(3, 1, 10, 1), 256)
	keep := thumbnails.Path(cache, key, 256)
	for _, p := range []string{orphan, fresh, keep, filepath.Join(cache, "original.jpg")} {
		os.MkdirAll(filepath.Dir(p), 0700)
		os.WriteFile(p, []byte("data"), 0600)
		if p != fresh {
			os.Chtimes(p, old, old)
		}
	}
	expired := filepath.Join(cache, "full-images-v1", key+".png")
	os.MkdirAll(filepath.Dir(expired), 0700)
	os.WriteFile(expired, []byte("data"), 0600)
	expiredAt := time.Now().Add(-31 * 24 * time.Hour)
	os.Chtimes(expired, expiredAt, expiredAt)
	if e = s.auditCache(context.Background(), true); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(orphan); !os.IsNotExist(e) {
		t.Fatal("orphan retained")
	}
	for _, p := range []string{fresh, keep, filepath.Join(cache, "original.jpg")} {
		if _, e = os.Stat(p); e != nil {
			t.Fatal("protected file removed", p)
		}
	}
	if s.cacheState.Requeued != 1 || s.cacheState.RemovedBytes != 8 {
		t.Fatal(s.cacheState)
	}
	if e = s.auditCache(context.Background(), true); e != nil {
		t.Fatal(e)
	}
	if s.cacheState.Requeued != 0 {
		t.Fatal("duplicate rebuild")
	}
	mux := http.NewServeMux()
	s.Register(mux)
	r := httptest.NewRequest("POST", "/api/cache", nil)
	r = r.WithContext(access.WithUser(r.Context(), access.User{ID: 2, Role: "member"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestVideoConversionAndAccess(t *testing.T) {
	if _, e := exec.LookPath("ffmpeg"); e != nil {
		t.Skip("ffmpeg required")
	}
	root := t.TempDir()
	source := filepath.Join(root, "sample.mkv")
	cmd := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=96x64:rate=10", "-t", "0.5", "-c:v", "ffv1", source)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	before, _ := os.ReadFile(source)
	st, _ := os.Stat(source)
	db, e := database.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	db.Exec("INSERT INTO users(id,username,display_name,password_hash,role) VALUES(1,'admin','Admin','','admin'),(2,'member','Member','','member')")
	if _, e = db.Exec("INSERT INTO libraries VALUES('photos','Photos',?,1)", root); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec("INSERT INTO assets(id,library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan) VALUES(1,'photos','sample.mkv','','sample.mkv','.mkv','video/x-matroska','video',?,?,'2026-01-01','scan')", st.Size(), st.ModTime().UnixNano()); e != nil {
		t.Fatal(e)
	}
	s := Server{DB: db, Cache: t.TempDir()}
	a := playbackAsset{1, 1, st.Size(), st.ModTime().UnixNano(), root, "sample.mkv"}
	if e = s.convertVideo(context.Background(), a); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(source)
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("original changed")
	}
	out, e := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=codec_name,width,height", "-of", "csv=p=0", filepath.Join(s.Cache, "video-v1", a.key()+".mp4")).Output()
	if e != nil || strings.TrimSpace(string(out)) != "h264,96,64" {
		t.Fatal(e, string(out))
	}
	db.Exec("INSERT INTO video_jobs(asset_id,generation,state) VALUES(1,1,'ready')")
	mux := http.NewServeMux()
	s.Register(mux)
	for _, tc := range []struct {
		role string
		id   int64
		want int
	}{{"admin", 1, 206}, {"member", 2, 404}} {
		r := httptest.NewRequest("GET", "/api/assets/1/playback/file", nil)
		r.Header.Set("Range", "bytes=0-15")
		r = r.WithContext(access.WithUser(r.Context(), access.User{ID: tc.id, Role: tc.role}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	db.Exec("UPDATE assets SET generation=2")
	if e = s.convertVideo(context.Background(), a); e == nil {
		t.Fatal("stale generation published")
	}
}
