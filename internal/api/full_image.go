package api

import (
	"database/sql"
	"fmt"
	"gallery/internal/access"
	"gallery/internal/library"
	"gallery/internal/metadata"
	"gallery/internal/thumbnails"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Only one full-size decode at a time: RAW development can use substantial RAM.
var fullImageSlot = make(chan struct{}, 1)

func (s *Server) fullImage(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r, "id")
	if e != nil {
		http.Error(w, "invalid asset", 400)
		return
	}
	var root, rel, kind string
	var generation, size, mtime int64
	e = s.DB.QueryRowContext(r.Context(), "SELECT l.root,a.relative_path,a.media_type,a.generation,a.file_size,a.modified_at FROM assets a JOIN libraries l ON l.id=a.library_id WHERE a.id=? AND l.enabled=1 AND "+access.ReadFile(access.Current(r), "a"), id).Scan(&root, &rel, &kind, &generation, &size, &mtime)
	if e == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	if kind == "video" {
		http.Error(w, "image required", 400)
		return
	}
	key := thumbnails.Key(id, generation, size, mtime)
	target := filepath.Join(s.Cache, "full-images-v1", key+".png")
	select {
	case fullImageSlot <- struct{}{}:
		defer func() { <-fullImageSlot }()
	case <-r.Context().Done():
		return
	}
	if _, e = os.Stat(target); os.IsNotExist(e) {
		if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
			s.fail(w, e)
			return
		}
		temp, e := os.MkdirTemp(filepath.Dir(target), "render-")
		if e != nil {
			s.fail(w, e)
			return
		}
		defer os.RemoveAll(temp)
		source, e := library.Open(root, rel)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		info, e := source.Stat()
		if e != nil || info.Size() != size || info.ModTime().UnixNano() != mtime {
			source.Close()
			http.Error(w, "Source changed; scan library first", 409)
			return
		}
		input := filepath.Join(temp, "source"+filepath.Ext(rel))
		copy, e := os.Create(input)
		if e != nil {
			source.Close()
			s.fail(w, e)
			return
		}
		_, e = io.Copy(copy, source)
		closeErr := copy.Close()
		source.Close()
		if e == nil {
			e = closeErr
		}
		if e != nil {
			s.fail(w, e)
			return
		}
		output := filepath.Join(temp, "full.png")
		// Decode the actual sensor data for RAW; never substitute an embedded JPEG.
		if kind == "raw" {
			input += "[bitdepth=16]"
		}
		_, e = (metadata.Command{}).Run(r.Context(), "vips", "autorot", input, output+"[compression=3,keep=icc]")
		if e != nil {
			http.Error(w, "Full-resolution rendering unavailable for this format on this server", 422)
			return
		}
		if e = os.Rename(output, target); e != nil {
			s.fail(w, e)
			return
		}
	} else if e != nil {
		s.fail(w, e)
		return
	}
	// Recheck access and generation after the potentially lengthy conversion.
	var allowed int
	e = s.DB.QueryRowContext(r.Context(), "SELECT 1 FROM assets a JOIN libraries l ON l.id=a.library_id WHERE a.id=? AND a.generation=? AND l.enabled=1 AND "+access.ReadFile(access.Current(r), "a"), id, generation).Scan(&allowed)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"image-%d.png\"", id))
	_ = os.Chtimes(target, time.Now(), time.Now())
	http.ServeFile(w, r, target)
}
