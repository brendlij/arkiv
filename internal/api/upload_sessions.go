package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gallery/internal/access"
	"gallery/internal/media"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const uploadChunkSize int64 = 8 << 20

type uploadRecord struct {
	ID       string `json:"id"`
	Owner    int64  `json:"-"`
	Key      string `json:"-"`
	Filename string `json:"filename"`
	Folder   string `json:"folder"`
	Size     int64  `json:"size"`
	Hash     string `json:"sha256"`
	Keep     bool   `json:"keepCopy"`
	Received int64  `json:"received"`
	State    string `json:"state"`
	AssetID  int64  `json:"assetId"`
	Updated  int64  `json:"updatedAt"`
	Trashed  bool   `json:"trashed"`
}

const uploadColumns = "id,owner_id,client_key,filename,folder,file_size,sha256,keep_copy,received,state,coalesce(asset_id,0),updated_at"

func readUpload(row rowScanner) (uploadRecord, error) {
	var v uploadRecord
	e := row.Scan(&v.ID, &v.Owner, &v.Key, &v.Filename, &v.Folder, &v.Size, &v.Hash, &v.Keep, &v.Received, &v.State, &v.AssetID, &v.Updated)
	return v, e
}
func (s *Server) retention() time.Duration {
	if s.UploadRetention > 0 {
		return s.UploadRetention
	}
	return 7 * 24 * time.Hour
}
func (s *Server) quota(ctx context.Context, uid int64) (int64, int64, int64, error) {
	var quota, used, reserved int64
	e := s.DB.QueryRowContext(ctx, "SELECT upload_quota FROM users WHERE id=?", uid).Scan(&quota)
	if e != nil {
		return 0, 0, 0, e
	}
	if quota == 0 {
		quota = s.UploadQuota
		if quota == 0 {
			quota = uploadQuota
		}
	}
	e = s.DB.QueryRowContext(ctx, "SELECT coalesce(sum(file_size),0) FROM assets WHERE NOT EXISTS(SELECT 1 FROM media_deletions md WHERE md.asset_id=assets.id) AND library_id='arkiv-uploads' AND relative_path LIKE ?", fmt.Sprintf("user-%d/%%", uid)).Scan(&used)
	if e != nil {
		return 0, 0, 0, e
	}
	var quarantined int64
	if e = s.DB.QueryRowContext(ctx, "SELECT coalesce(sum(file_size),0) FROM upload_quarantine WHERE owner_id=?", uid).Scan(&quarantined); e != nil {
		return 0, 0, 0, e
	}
	used += quarantined
	e = s.DB.QueryRowContext(ctx, "SELECT coalesce(sum(file_size),0) FROM upload_sessions WHERE owner_id=? AND (state='committing' OR (state='uploading' AND updated_at>?))", uid, time.Now().Add(-s.retention()).Unix()).Scan(&reserved)
	return quota, used, reserved, e
}
func uploadUser(w http.ResponseWriter, r *http.Request) (access.User, bool) {
	u := access.Current(r)
	if u.ID < 1 || u.PublicAlbum != 0 {
		http.Error(w, "Sign in to upload", 401)
		return u, false
	}
	return u, true
}
func uploadBusy(w http.ResponseWriter) bool {
	if !uploadLock.TryLock() {
		w.Header().Set("Retry-After", "2")
		http.Error(w, "Another upload operation is finishing. Retry shortly.", 429)
		return true
	}
	return false
}
func validDigest(v string) bool {
	b, e := hex.DecodeString(v)
	return e == nil && len(b) == 32 && v == strings.ToLower(v)
}
func (s *Server) getUpload(r *http.Request) (uploadRecord, error) {
	return readUpload(s.DB.QueryRowContext(r.Context(), "SELECT "+uploadColumns+" FROM upload_sessions WHERE id=? AND owner_id=?", r.PathValue("uploadID"), access.Current(r).ID))
}
func (s *Server) replyUpload(w http.ResponseWriter, v uploadRecord) {
	if v.AssetID > 0 {
		var n int
		s.DB.QueryRow("SELECT trashed_at FROM assets WHERE id=?", v.AssetID).Scan(&n)
		v.Trashed = n > 0
	}
	send(w, v)
}

func (s *Server) startUpload(w http.ResponseWriter, r *http.Request) {
	u, ok := uploadUser(w, r)
	if !ok {
		return
	}
	if s.Uploads == "" {
		http.Error(w, "Uploads unavailable", 503)
		return
	}
	var b struct {
		Filename string `json:"filename"`
		Folder   string `json:"folder"`
		Size     int64  `json:"size"`
		Hash     string `json:"sha256"`
		Key      string `json:"clientKey"`
		Keep     bool   `json:"keepCopy"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&b) != nil || !validDigest(b.Hash) || len(b.Key) < 16 || len(b.Key) > 100 || strings.ContainsAny(b.Key, "\x00\r\n") {
		http.Error(w, "Invalid upload identity", 400)
		return
	}
	if b.Filename == "" || len(b.Filename) > 180 || strings.ContainsAny(b.Filename, "\\/:\x00") || strings.HasPrefix(b.Filename, ".") {
		http.Error(w, "Invalid filename", 400)
		return
	}
	format, supported := media.Lookup(strings.ToLower(filepath.Ext(b.Filename)))
	limit := media.ImageUploadLimit
	if format.Kind == "video" {
		limit = media.VideoUploadLimit
	}
	if !supported || b.Size <= 0 || b.Size > limit {
		http.Error(w, "Unsupported format or file size", 415)
		return
	}
	folder, e := uploadPath(u.ID, b.Folder)
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if uploadBusy(w) {
		return
	}
	defer uploadLock.Unlock()
	old, e := readUpload(s.DB.QueryRowContext(r.Context(), "SELECT "+uploadColumns+" FROM upload_sessions WHERE owner_id=? AND client_key=?", u.ID, b.Key))
	if e == nil {
		if old.Hash != b.Hash || old.Size != b.Size || old.Folder != folder || old.Filename != b.Filename || old.Keep != b.Keep {
			http.Error(w, "Upload key belongs to a different file or destination", 409)
			return
		}
		s.replyUpload(w, old)
		return
	}
	if e != sql.ErrNoRows {
		s.fail(w, e)
		return
	}
	root, e := os.OpenRoot(s.Uploads)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer root.Close()
	var duplicate int64
	if !b.Keep {
		duplicate, e = s.findDuplicate(r.Context(), root, u.ID, b.Size, b.Hash)
		if e != nil {
			s.fail(w, e)
			return
		}
	}
	quota, used, reserved, e := s.quota(r.Context(), u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	if duplicate == 0 && b.Size > quota-used-reserved {
		http.Error(w, "Not enough storage. Discard paused uploads or ask an administrator to increase your limit.", 413)
		return
	}
	var pending int
	if e = s.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM upload_sessions WHERE owner_id=? AND (state='committing' OR (state='uploading' AND updated_at>?))", u.ID, time.Now().Add(-s.retention()).Unix()).Scan(&pending); e != nil {
		s.fail(w, e)
		return
	}
	if pending >= 50 {
		http.Error(w, "Finish or discard paused uploads first (50 active uploads maximum).", 429)
		return
	}
	id := fmt.Sprintf("%x", rand.Text())
	state := "uploading"
	received := int64(0)
	var asset any
	if duplicate > 0 {
		state = "duplicate"
		received = b.Size
		asset = duplicate
	}
	if _, e = s.DB.ExecContext(r.Context(), "INSERT INTO upload_sessions(id,owner_id,client_key,filename,folder,file_size,sha256,keep_copy,received,state,asset_id,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", id, u.ID, b.Key, b.Filename, folder, b.Size, b.Hash, b.Keep, received, state, asset, time.Now().Unix()); e != nil {
		s.fail(w, e)
		return
	}
	v, e := readUpload(s.DB.QueryRowContext(r.Context(), "SELECT "+uploadColumns+" FROM upload_sessions WHERE id=?", id))
	if e != nil {
		s.fail(w, e)
		return
	}
	s.replyUpload(w, v)
}

// Old uploads are hashed lazily only when their size matches a new file. The
// digest is never accepted from a client as evidence about stored original bytes.
func (s *Server) findDuplicate(ctx context.Context, root *os.Root, uid, size int64, digest string) (int64, error) {
	rows, e := s.DB.QueryContext(ctx, "SELECT id,relative_path,coalesce(content_hash,'') FROM assets WHERE NOT EXISTS(SELECT 1 FROM media_deletions md WHERE md.asset_id=assets.id) AND library_id='arkiv-uploads' AND relative_path LIKE ? AND file_size=? AND (content_hash=? OR coalesce(content_hash,'')='') ORDER BY trashed_at,id", fmt.Sprintf("user-%d/%%", uid), size, digest)
	if e != nil {
		return 0, e
	}
	type candidate struct {
		id         int64
		path, hash string
	}
	all := []candidate{}
	for rows.Next() {
		var c candidate
		if e = rows.Scan(&c.id, &c.path, &c.hash); e != nil {
			rows.Close()
			return 0, e
		}
		all = append(all, c)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	for _, c := range all {
		if c.hash == "" {
			f, err := root.Open(c.path)
			if err != nil {
				continue
			}
			h := sha256.New()
			n, err := io.Copy(h, contextReader{ctx, f})
			f.Close()
			if err != nil {
				return 0, err
			}
			if n != size {
				continue
			}
			c.hash = fmt.Sprintf("%x", h.Sum(nil))
			if _, e = s.DB.ExecContext(ctx, "UPDATE assets SET content_hash=? WHERE id=?", c.hash, c.id); e != nil {
				return 0, e
			}
		}
		if c.hash == digest {
			return c.id, nil
		}
	}
	return 0, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(p)
}

func (s *Server) listUploads(w http.ResponseWriter, r *http.Request) {
	u, ok := uploadUser(w, r)
	if !ok {
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), "SELECT "+uploadColumns+" FROM upload_sessions WHERE owner_id=? AND state IN ('uploading','committing') AND (state='committing' OR updated_at>?) ORDER BY updated_at DESC LIMIT 50", u.ID, time.Now().Add(-s.retention()).Unix())
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	items := []uploadRecord{}
	for rows.Next() {
		v, err := readUpload(rows)
		if err != nil {
			s.fail(w, err)
			return
		}
		items = append(items, v)
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	send(w, items)
}
func (s *Server) uploadSession(w http.ResponseWriter, r *http.Request) {
	if _, ok := uploadUser(w, r); !ok {
		return
	}
	v, e := s.getUpload(r)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	if v.State == "uploading" && v.Updated < time.Now().Add(-s.retention()).Unix() {
		http.Error(w, "Upload expired. Start again.", 410)
		return
	}
	s.replyUpload(w, v)
}

func (s *Server) uploadChunk(w http.ResponseWriter, r *http.Request) {
	if _, ok := uploadUser(w, r); !ok {
		return
	}
	v, e := s.getUpload(r)
	if e != nil {
		http.NotFound(w, r)
		return
	}

	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(2 * time.Minute))
	data, e := io.ReadAll(http.MaxBytesReader(w, r.Body, uploadChunkSize))
	if e != nil || len(data) == 0 {
		http.Error(w, "Invalid or oversized upload chunk", 400)
		return
	}
	if uploadBusy(w) {
		return
	}
	defer uploadLock.Unlock()
	v, e = s.getUpload(r)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	offset, e := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if e != nil || v.State != "uploading" || offset != v.Received {
		http.Error(w, "Offset changed. Read upload status before resuming.", 409)
		return
	}
	if v.Updated < time.Now().Add(-s.retention()).Unix() {
		http.Error(w, "Upload expired. Start again.", 410)
		return
	}
	if int64(len(data)) > v.Size-v.Received {
		http.Error(w, "Chunk exceeds remaining bytes", 400)
		return
	}
	root, e := os.OpenRoot(s.Uploads)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer root.Close()
	if e = root.MkdirAll(".incoming", 0700); e != nil {
		s.fail(w, e)
		return
	}
	f, e := root.OpenFile(".incoming/"+v.ID+".part", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		s.fail(w, e)
		return
	}
	if info.Size() < v.Received {
		http.Error(w, "Staged data is missing. Discard this upload and start again.", 409)
		return
	}
	// Bytes written before a crash but not acknowledged in SQLite are discarded.
	if e = f.Truncate(v.Received); e == nil {
		_, e = f.Seek(v.Received, 0)
	}
	if e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	now := time.Now().Unix()
	if _, e = s.DB.ExecContext(r.Context(), "UPDATE upload_sessions SET received=?,updated_at=? WHERE id=?", v.Received+int64(len(data)), now, v.ID); e != nil {
		s.fail(w, e)
		return
	}
	v.Received += int64(len(data))
	v.Updated = now
	s.replyUpload(w, v)
}

func (s *Server) finishUpload(ctx context.Context, v uploadRecord) (uploadRecord, error) {
	if v.State == "done" || v.State == "duplicate" {
		return v, nil
	}
	if v.Received != v.Size {
		return v, fmt.Errorf("Upload is incomplete")
	}
	root, e := os.OpenRoot(s.Uploads)
	if e != nil {
		return v, e
	}
	defer root.Close()
	ext := strings.ToLower(filepath.Ext(v.Filename))
	temp := ".incoming/" + v.ID + ".part"
	dest := fmt.Sprintf("user-%d/%s%s", v.Owner, v.ID, ext)
	filePath := temp
	if v.State == "committing" {
		if _, e = root.Stat(dest); e == nil {
			filePath = dest
		} else if !os.IsNotExist(e) {
			return v, e
		}
	}
	f, e := root.Open(filePath)
	if e != nil {
		return v, e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, contextReader{ctx, f})
	if e != nil {
		return v, e
	}
	if n != v.Size || fmt.Sprintf("%x", h.Sum(nil)) != v.Hash {
		return v, fmt.Errorf("File integrity check failed. Discard the upload and select the original file again")
	}
	if _, e = f.Seek(0, 0); e != nil {
		return v, e
	}
	dimensions, e := media.ValidateUpload(f, ext)
	if e != nil {
		return v, e
	}
	info, e := f.Stat()
	if e != nil {
		return v, e
	}
	f.Close()
	if !v.Keep {
		id, err := s.findDuplicate(ctx, root, v.Owner, v.Size, v.Hash)
		if err != nil {
			return v, err
		}
		if id > 0 {
			_, e = s.DB.ExecContext(ctx, "UPDATE upload_sessions SET state='duplicate',asset_id=?,updated_at=? WHERE id=?", id, time.Now().Unix(), v.ID)
			if e != nil {
				return v, e
			}
			root.Remove(filePath)
			v.State = "duplicate"
			v.AssetID = id
			return v, nil
		}
	}
	if _, e = s.DB.ExecContext(ctx, "UPDATE upload_sessions SET state='committing',updated_at=? WHERE id=?", time.Now().Unix(), v.ID); e != nil {
		return v, e
	}
	// The durable committing record protects renamed bytes across a process crash.
	if filePath == temp {
		if e = root.MkdirAll(fmt.Sprintf("user-%d", v.Owner), 0700); e != nil {
			return v, e
		}
		if e = root.Rename(temp, dest); e != nil {
			return v, e
		}
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return v, e
	}
	defer tx.Rollback()
	if e = ensureUploadFolder(ctx, tx, v.Owner, v.Folder); e != nil {
		return v, e
	}
	format, _ := media.Lookup(ext)
	result, e := tx.ExecContext(ctx, `INSERT INTO assets(library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan,width,height,content_hash) VALUES('arkiv-uploads',?,?,?,?,?,?,?,?,?,'upload',?,?,?)`, dest, v.Folder, v.Filename, ext, format.MIME, format.Kind, v.Size, info.ModTime().UnixNano(), time.Now().UTC().Format(time.RFC3339), dimensions.Width, dimensions.Height, v.Hash)
	if e != nil {
		return v, e
	}
	id, e := result.LastInsertId()
	if e != nil {
		return v, e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO jobs(asset_id) VALUES(?)", id); e != nil {
		return v, e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE upload_sessions SET state='done',asset_id=?,updated_at=? WHERE id=?", id, time.Now().Unix(), v.ID); e != nil {
		return v, e
	}
	if e = tx.Commit(); e != nil {
		return v, e
	}
	v.State = "done"
	v.AssetID = id
	return v, nil
}
func (s *Server) completeUpload(w http.ResponseWriter, r *http.Request) {
	if _, ok := uploadUser(w, r); !ok {
		return
	}
	if uploadBusy(w) {
		return
	}
	defer uploadLock.Unlock()
	v, e := s.getUpload(r)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	if v.State == "uploading" && v.Updated < time.Now().Add(-s.retention()).Unix() {
		http.Error(w, "Upload expired", 410)
		return
	}
	v, e = s.finishUpload(r.Context(), v)
	if e != nil {
		http.Error(w, "Could not finalize upload: "+e.Error(), 409)
		return
	}
	s.replyUpload(w, v)
}
func (s *Server) cancelUpload(w http.ResponseWriter, r *http.Request) {
	if _, ok := uploadUser(w, r); !ok {
		return
	}
	if uploadBusy(w) {
		return
	}
	defer uploadLock.Unlock()
	v, e := s.getUpload(r)
	if errors.Is(e, sql.ErrNoRows) {
		w.WriteHeader(204)
		return
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	if v.State == "committing" {
		http.Error(w, "Upload is being finalized. Resume to finish it.", 409)
		return
	}
	if v.State == "uploading" {
		root, err := os.OpenRoot(s.Uploads)
		if err != nil {
			s.fail(w, err)
			return
		}
		e = root.Remove(".incoming/" + v.ID + ".part")
		root.Close()
		if e != nil && !os.IsNotExist(e) {
			s.fail(w, e)
			return
		}
	}
	if _, e = s.DB.ExecContext(r.Context(), "DELETE FROM upload_sessions WHERE id=?", v.ID); e != nil {
		s.fail(w, e)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) userStorage(w http.ResponseWriter, r *http.Request) {
	if !access.Admin(w, r) {
		return
	}
	uid, e := idParam(r, "userId")
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if r.Method == "POST" {
		var b struct {
			Quota int64 `json:"quotaBytes"`
		}
		if json.NewDecoder(r.Body).Decode(&b) != nil || b.Quota < 0 || b.Quota > 1<<50 {
			http.Error(w, "Invalid storage limit", 400)
			return
		}
		uploadLock.Lock()
		_, e = s.DB.ExecContext(r.Context(), "UPDATE users SET upload_quota=? WHERE id=?", b.Quota, uid)
		uploadLock.Unlock()
		if e != nil {
			s.fail(w, e)
			return
		}
	}
	quota, used, reserved, e := s.quota(r.Context(), uid)
	if e == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	var override int64
	s.DB.QueryRowContext(r.Context(), "SELECT upload_quota FROM users WHERE id=?", uid).Scan(&override)
	send(w, map[string]any{"quota": quota, "used": used, "reserved": reserved, "override": override})
}
