package api

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"gallery/internal/access"
	"net/http"
	"strconv"
	"strings"
)

type Asset struct {
	PlaceID        string   `json:"placeId"`
	PlaceName      string   `json:"placeName"`
	ID             int64    `json:"id"`
	LibraryID      string   `json:"libraryId"`
	RelativePath   string   `json:"relativePath"`
	Folder         string   `json:"folder"`
	Filename       string   `json:"filename"`
	MediaType      string   `json:"mediaType"`
	MimeType       string   `json:"mimeType"`
	FileSize       int64    `json:"fileSize"`
	TakenAt        string   `json:"takenAt"`
	Width          int      `json:"width"`
	Height         int      `json:"height"`
	Orientation    int      `json:"orientation"`
	CameraMake     string   `json:"cameraMake"`
	CameraModel    string   `json:"cameraModel"`
	Lens           string   `json:"lens"`
	FocalLength    string   `json:"focalLength"`
	Aperture       string   `json:"aperture"`
	ExposureTime   string   `json:"exposureTime"`
	ISO            string   `json:"iso"`
	Latitude       *float64 `json:"latitude"`
	Longitude      *float64 `json:"longitude"`
	Duration       float64  `json:"duration"`
	VideoCodec     string   `json:"videoCodec"`
	AudioCodec     string   `json:"audioCodec"`
	CanManage      bool     `json:"canManage"`
	CanAdd         bool     `json:"canAdd"`
	CanRemove      bool     `json:"canRemove"`
	Favorite       bool     `json:"favorite"`
	PreviewStatus  string   `json:"previewStatus"`
	MetadataStatus string   `json:"metadataStatus"`
	Error          string   `json:"error"`
}

const assetCols = `a.id,a.library_id,a.relative_path,a.folder,a.filename,a.media_type,a.mime_type,a.file_size,a.taken_at,a.width,a.height,a.orientation,a.camera_make,a.camera_model,a.lens,a.focal_length,a.aperture,a.exposure_time,a.iso,a.latitude,a.longitude,a.duration,a.video_codec,a.audio_codec,a.favorite,a.preview_status,a.metadata_status,a.error,a.place_id,a.place_name`

type rowScanner interface{ Scan(...any) error }

func readAsset(row rowScanner) (Asset, error) {
	var a Asset
	e := row.Scan(&a.ID, &a.LibraryID, &a.RelativePath, &a.Folder, &a.Filename, &a.MediaType, &a.MimeType, &a.FileSize, &a.TakenAt, &a.Width, &a.Height, &a.Orientation, &a.CameraMake, &a.CameraModel, &a.Lens, &a.FocalLength, &a.Aperture, &a.ExposureTime, &a.ISO, &a.Latitude, &a.Longitude, &a.Duration, &a.VideoCodec, &a.AudioCodec, &a.Favorite, &a.PreviewStatus, &a.MetadataStatus, &a.Error, &a.PlaceID, &a.PlaceName, &a.CanAdd, &a.CanRemove, &a.CanManage)
	return a, e
}

type cursor struct {
	Date string `json:"d"`
	ID   int64  `json:"i"`
}

func (s *Server) assets(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 120
	if v := q.Get("limit"); v != "" {
		var e error
		limit, e = strconv.Atoi(v)
		if e != nil || limit < 1 || limit > 240 {
			http.Error(w, "limit must be 1..240", 400)
			return
		}
	}
	where := []string{"l.enabled=1", access.Visible(access.Current(r), "a")}
	if q.Get("trash") == "true" {
		where = []string{"l.enabled=1", access.Trash(access.Current(r), "a")}
	}
	args := []any{}
	if v := q.Get("place"); v != "" {
		where = append(where, "a.place_id=?")
		args = append(args, v)
	}
	if v := q.Get("bbox"); v != "" {
		box, e := parseBounds(v)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		condition, values := box.sql()
		where = append(where, condition)
		args = append(args, values...)
	}
	if v := q.Get("cursor"); v != "" {
		b, e := base64.RawURLEncoding.DecodeString(v)
		var c cursor
		if e != nil || json.Unmarshal(b, &c) != nil || c.ID < 1 || c.Date == "" {
			http.Error(w, "invalid cursor", 400)
			return
		}
		where = append(where, "(a.taken_at<? OR (a.taken_at=? AND a.id<?))")
		args = append(args, c.Date, c.Date, c.ID)
	}
	if q.Get("favorite") == "true" {
		where = append(where, access.Favorite(access.Current(r)))
	}
	if v := q.Get("library"); v != "" {
		where = append(where, "a.library_id=?")
		args = append(args, v)
	}
	if q.Has("folder") {
		where = append(where, "a.folder=?")
		args = append(args, q.Get("folder"))
	}
	if q.Get("inbox") == "true" {
		where = append(where, "(lower(a.folder)='inbox' OR lower(a.folder) LIKE 'inbox/%')")
	}
	if v := q.Get("album"); v != "" {
		id, e := strconv.ParseInt(v, 10, 64)
		if e != nil || id < 1 {
			http.Error(w, "invalid album", 400)
			return
		}
		if !s.albumAllowed(w, r, id, false) {
			return
		}
		where = append(where, "EXISTS(SELECT 1 FROM album_assets aa WHERE aa.asset_id=a.id AND aa.album_id=?)")
		args = append(args, id)
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		if len(v) > 200 {
			http.Error(w, "search too long", 400)
			return
		}
		var terms []string
		for _, t := range strings.Fields(v) {
			terms = append(terms, `"`+strings.ReplaceAll(t, `"`, `""`)+`"*`)
		}
		where = append(where, "a.id IN (SELECT rowid FROM assets_fts WHERE assets_fts MATCH ?)")
		args = append(args, strings.Join(terms, " AND "))
	}
	args = append(args, limit+1)
	rows, e := s.DB.QueryContext(r.Context(), "SELECT "+assetColumns(r)+" FROM assets a JOIN libraries l ON l.id=a.library_id WHERE "+strings.Join(where, " AND ")+" ORDER BY a.taken_at DESC,a.id DESC LIMIT ?", args...)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	items := []Asset{}
	for rows.Next() {
		a, e := readAsset(rows)
		if e != nil {
			s.fail(w, e)
			return
		}
		items = append(items, a)
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		a := items[len(items)-1]
		b, _ := json.Marshal(cursor{a.TakenAt, a.ID})
		next = base64.RawURLEncoding.EncodeToString(b)
	}
	send(w, map[string]any{"items": items, "nextCursor": next})
}
func (s *Server) asset(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r, "id")
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	a, e := readAsset(s.DB.QueryRowContext(r.Context(), "SELECT "+assetColumns(r)+" FROM assets a JOIN libraries l ON l.id=a.library_id WHERE a.id=? AND l.enabled=1 AND "+access.ReadFile(access.Current(r), "a"), id))
	if e == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	send(w, a)
}
func idParam(r *http.Request, name string) (int64, error) {
	id, e := strconv.ParseInt(r.PathValue(name), 10, 64)
	if e != nil || id < 1 {
		return 0, fmt.Errorf("invalid %s", name)
	}
	return id, nil
}

func assetColumns(r *http.Request) string {
	s := strings.Replace(assetCols, "a.favorite", access.Favorite(access.Current(r)), 1)
	if access.Current(r).Role != "admin" {
		s = strings.Replace(s, "a.error", "''", 1)
	}

	u := access.Current(r)
	remove := "0"
	if id, e := strconv.ParseInt(r.URL.Query().Get("album"), 10, 64); e == nil && id > 0 {
		remove = fmt.Sprintf("EXISTS(SELECT 1 FROM album_assets aa JOIN albums al ON al.id=aa.album_id WHERE aa.asset_id=a.id AND aa.album_id=%d AND (aa.added_by=%d OR al.owner_id=%d OR %t))", id, u.ID, u.ID, u.Role == "admin")
	}
	return s + "," + access.Direct(u, "a") + "," + remove + "," + access.Manage(u, "a")
}
