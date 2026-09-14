package api

import (
	"database/sql"
	"gallery/internal/access"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) folders(w http.ResponseWriter, r *http.Request) {
	lib := r.URL.Query().Get("library")
	parent := strings.Trim(r.URL.Query().Get("parent"), "/")
	if len(parent) > 4096 {
		http.Error(w, "path too long", 400)
		return
	}
	if lib == "arkiv-uploads" {
		s.managedFolderList(w, r, parent)
		return
	}
	type folder struct {
		LibraryID string `json:"libraryId"`
		Name      string `json:"name"`
		Path      string `json:"path"`
		Count     int    `json:"count"`
	}
	items := []folder{}
	var rows *sql.Rows
	var e error
	offset := 0
	if r.URL.Query().Get("offset") != "" {
		offset, e = strconv.Atoi(r.URL.Query().Get("offset"))
		if e != nil || offset < 0 {
			http.Error(w, "invalid offset", 400)
			return
		}
	}
	if lib == "" {
		rows, e = s.DB.QueryContext(r.Context(), `SELECT l.id,l.name,'',count(a.id) FROM libraries l JOIN assets a ON a.library_id=l.id WHERE l.enabled=1 AND `+access.Direct(access.Current(r), "a")+` GROUP BY l.id ORDER BY l.name,l.id LIMIT 201 OFFSET ?`, offset)
	} else {
		prefix := ""
		if parent != "" {
			prefix = parent + "/"
		}
		rows, e = s.DB.QueryContext(r.Context(), `WITH descendants AS (SELECT library_id,substr(folder,?) AS rest FROM assets a WHERE `+access.Direct(access.Current(r), "a")+` AND library_id=? AND library_id IN(SELECT id FROM libraries WHERE enabled=1) AND substr(folder,1,?)=? AND folder<>?),children AS (SELECT library_id,CASE WHEN instr(rest,'/')=0 THEN rest ELSE substr(rest,1,instr(rest,'/')-1) END AS child FROM descendants) SELECT library_id,child,?||child,count(*) FROM children WHERE child<>'' GROUP BY library_id,child ORDER BY child LIMIT 201 OFFSET ?`, len([]rune(prefix))+1, lib, len([]rune(prefix)), prefix, parent, prefix, offset)
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var f folder
		if e = rows.Scan(&f.LibraryID, &f.Name, &f.Path, &f.Count); e != nil {
			s.fail(w, e)
			return
		}
		items = append(items, f)
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	more := len(items) > 200
	if more {
		items = items[:200]
	}
	send(w, map[string]any{"items": items, "hasMore": more})
}
