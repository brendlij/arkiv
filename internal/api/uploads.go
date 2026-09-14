package api

import (
	"crypto/rand"
	"fmt"
	"gallery/internal/access"
	"gallery/internal/media"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Bound disk pressure and serialize quota checks across upload requests.
var uploadLock sync.Mutex

const uploadLimit = media.VideoUploadLimit
const uploadQuota = media.UploadQuota

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	u := access.Current(r)
	if u.ID < 1 || u.PublicAlbum != 0 {
		http.Error(w, "Sign in to upload", 401)
		return
	}
	if s.Uploads == "" {
		http.Error(w, "Uploads are not configured", 503)
		return
	}
	if !uploadLock.TryLock() {
		http.Error(w, "Another upload is being saved. Please retry.", 429)
		return
	}
	defer uploadLock.Unlock()
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(time.Hour))
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit+(1<<20))
	reader, e := r.MultipartReader()
	if e != nil {
		http.Error(w, "Expected a media upload", 400)
		return
	}
	part, e := reader.NextPart()
	if e != nil || part.FormName() != "file" {
		http.Error(w, "Choose one media file per request", 400)
		return
	}
	_, disposition, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
	if err != nil {
		http.Error(w, "Invalid filename", 400)
		return
	}
	name := disposition["filename"]
	if name == "" || len(name) > 180 || strings.ContainsAny(name, "\\/:\x00") || strings.HasPrefix(name, ".") {
		http.Error(w, "Invalid filename", 400)
		return
	}
	ext := strings.ToLower(filepath.Ext(name))
	format, supported := media.Lookup(ext)
	if !supported {
		http.Error(w, "Unsupported format. See the supported photo, RAW and video formats in the upload dialog.", 415)
		return
	}
	folder := fmt.Sprintf("user-%d", u.ID)
	destinationFolder, e := uploadPath(u.ID, r.URL.Query().Get("folder"))
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	quota, used, reserved, e := s.quota(r.Context(), u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	if used+reserved >= quota {
		http.Error(w, "Your upload storage allowance is full", 413)
		return
	}
	root, e := os.OpenRoot(s.Uploads)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer root.Close()
	if e = root.MkdirAll(folder, 0700); e != nil {
		s.fail(w, e)
		return
	}
	token := fmt.Sprintf("%x", rand.Text())
	temp := folder + "/" + token + ".partial"
	dest := folder + "/" + token + ext
	f, e := root.OpenFile(temp, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer func() { f.Close(); root.Remove(temp) }()
	limit := media.ImageUploadLimit
	if format.Kind == "video" {
		limit = media.VideoUploadLimit
	}
	if remaining := quota - used - reserved; remaining < limit {
		limit = remaining
	}
	n, e := io.Copy(f, io.LimitReader(part, limit+1))
	if e != nil || n > limit || n+used+reserved > quota {
		http.Error(w, "Upload incomplete or exceeds the 250 MiB photo/RAW, 2 GiB video, or account storage limit", 413)
		return
	}
	if _, e = reader.NextPart(); e != io.EOF {
		http.Error(w, "Send one media file per request", 400)
		return
	}
	if _, e = f.Seek(0, 0); e != nil {
		s.fail(w, e)
		return
	}
	dimensions, e := media.ValidateUpload(f, ext)
	if e != nil {
		http.Error(w, e.Error(), 415)
		return
	}
	if e = f.Sync(); e != nil {
		s.fail(w, e)
		return
	}
	if e = f.Close(); e != nil {
		s.fail(w, e)
		return
	}
	info, e := root.Stat(temp)
	if e != nil {
		s.fail(w, e)
		return
	}
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(r.Context(), "INSERT INTO folder_grants(user_id,library_id,folder,label) VALUES(?,'arkiv-uploads',?,'My uploads') ON CONFLICT(user_id,library_id,folder) DO NOTHING", u.ID, folder)
	if e != nil {
		s.fail(w, e)
		return
	}
	if e = ensureUploadFolder(r.Context(), tx, u.ID, destinationFolder); e != nil {
		s.fail(w, e)
		return
	}
	result, e := tx.ExecContext(r.Context(), `INSERT INTO assets(library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan,width,height) VALUES('arkiv-uploads',?,?,?,?,?,?,?,?,?,'upload',?,?)`, dest, destinationFolder, name, ext, format.MIME, format.Kind, n, info.ModTime().UnixNano(), time.Now().UTC().Format(time.RFC3339), dimensions.Width, dimensions.Height)
	if e != nil {
		s.fail(w, e)
		return
	}
	id, e := result.LastInsertId()
	if e != nil {
		s.fail(w, e)
		return
	}
	if _, e = tx.ExecContext(r.Context(), "INSERT INTO jobs(asset_id) VALUES(?)", id); e != nil {
		s.fail(w, e)
		return
	}
	if e = root.Rename(temp, dest); e != nil {
		s.fail(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		root.Remove(dest)
		s.fail(w, e)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	send(w, map[string]any{"id": id, "filename": name, "status": "processing"})
}

func (s *Server) uploadOptions(w http.ResponseWriter, r *http.Request) {
	u, ok := uploadUser(w, r)
	if !ok {
		return
	}
	quota, used, reserved, e := s.quota(r.Context(), u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	send(w, map[string]any{"formats": media.Formats(), "imageLimit": media.ImageUploadLimit, "videoLimit": media.VideoUploadLimit, "quota": quota, "used": used, "reserved": reserved, "userId": u.ID, "chunkSize": uploadChunkSize, "retentionHours": s.retention().Hours(), "enabled": s.Uploads != ""})
}
