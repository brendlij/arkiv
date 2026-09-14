package api

import (
	"encoding/json"
	"fmt"
	"gallery/internal/access"
	"log/slog"
	"net/http"
	"strings"
)

// Validate the complete selection before changing it. Mounted originals are never modified.
func (s *Server) bulkAssets(w http.ResponseWriter, r *http.Request) {
	u := access.Current(r)
	if u.ID < 1 || u.PublicAlbum != 0 {
		http.Error(w, "Sign in to manage media", 401)
		return
	}
	var in struct {
		IDs     []int64 `json:"ids"`
		Action  string  `json:"action"`
		Folder  string  `json:"folder"`
		Confirm bool    `json:"confirm"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768)).Decode(&in) != nil || len(in.IDs) == 0 || len(in.IDs) > 240 {
		http.Error(w, "Select between 1 and 240 items", 400)
		return
	}
	if in.Action != "move" && in.Action != "trash" && in.Action != "restore" && in.Action != "delete" {
		http.Error(w, "Unknown action", 400)
		return
	}
	if in.Action == "delete" && !in.Confirm {
		http.Error(w, "Confirm permanent deletion", 400)
		return
	}
	in.Folder = strings.TrimSpace(in.Folder)
	if in.Action == "move" && in.Folder != "" {
		if len(in.Folder) > 240 || strings.ContainsAny(in.Folder, "\\:\x00\r\n") {
			http.Error(w, "Invalid folder name", 400)
			return
		}
		for _, part := range strings.Split(in.Folder, "/") {
			if part == "" || part == "." || part == ".." || strings.TrimSpace(part) != part {
				http.Error(w, "Use folder names separated by /", 400)
				return
			}
		}
	}
	uploadLock.Lock()
	defer uploadLock.Unlock()
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	seen := map[int64]bool{}
	owned := map[int64]bool{}
	for _, id := range in.IDs {
		if id < 1 || seen[id] {
			http.Error(w, "Invalid or duplicate selection", 400)
			return
		}
		seen[id] = true
		var deleted int64
		allowed := access.Visible(u, "a")
		if in.Action == "move" {
			allowed = access.Manage(u, "a")
		}
		if in.Action == "restore" || in.Action == "delete" {
			allowed = access.Trash(u, "a")
		}
		var owner bool
		e = tx.QueryRowContext(r.Context(), "SELECT a.trashed_at,"+access.Manage(u, "a")+" FROM assets a JOIN libraries l ON l.id=a.library_id WHERE a.id=? AND l.enabled=1 AND ("+allowed+")", id).Scan(&deleted, &owner)
		owned[id] = owner
		if e != nil {
			http.Error(w, "The selection is unavailable or you do not have permission for this action", 403)
			return
		}
		if in.Action == "move" && deleted > 0 {
			http.Error(w, "Restore items before moving them", 409)
			return
		}
	}
	for _, id := range in.IDs {
		switch in.Action {
		case "trash":
			if owned[id] {
				_, e = tx.ExecContext(r.Context(), "UPDATE assets SET trashed_at=unixepoch() WHERE id=?", id)
			} else {
				_, e = tx.ExecContext(r.Context(), "INSERT INTO personal_trash(user_id,asset_id) VALUES(?,?) ON CONFLICT(user_id,asset_id) DO UPDATE SET dismissed=0", u.ID, id)
			}
		case "restore":
			if owned[id] {
				_, e = tx.ExecContext(r.Context(), "UPDATE assets SET trashed_at=0 WHERE id=?", id)
			} else {
				_, e = tx.ExecContext(r.Context(), "DELETE FROM personal_trash WHERE user_id=? AND asset_id=?", u.ID, id)
			}
		case "delete":
			if owned[id] {
				_, e = tx.ExecContext(r.Context(), "INSERT INTO media_deletions(asset_id) VALUES(?)", id)
				if e == nil {
					_, e = tx.ExecContext(r.Context(), "UPDATE assets SET generation=generation+1 WHERE id=?", id)
				}
				if e == nil {
					_, e = tx.ExecContext(r.Context(), "DELETE FROM jobs WHERE asset_id=?", id)
				}
			} else {
				_, e = tx.ExecContext(r.Context(), "UPDATE personal_trash SET dismissed=1 WHERE user_id=? AND asset_id=?", u.ID, id)
			}
		case "move":
			folder := fmt.Sprintf("user-%d", u.ID)
			if in.Folder != "" {
				folder += "/" + in.Folder
			}
			e = ensureUploadFolder(r.Context(), tx, u.ID, folder)
			if e == nil {
				_, e = tx.ExecContext(r.Context(), "UPDATE assets SET folder=? WHERE id=?", folder, id)
			}
		}
		if e != nil {
			s.fail(w, e)
			return
		}
	}
	if e = tx.Commit(); e != nil {
		s.fail(w, e)
		return
	}
	if in.Action == "delete" {
		if err := s.finishMediaDeletions(r.Context()); err != nil {
			slog.Warn("media deletion will retry during maintenance", "error", err)
		}
	}
	send(w, map[string]any{"updated": len(in.IDs)})
}

func (s *Server) uploadFolders(w http.ResponseWriter, r *http.Request) {
	u := access.Current(r)
	if u.ID < 1 {
		http.Error(w, "Sign in to manage folders", 401)
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), "SELECT path FROM managed_folders WHERE owner_id=? ORDER BY path", u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	folders := []string{}
	prefix := fmt.Sprintf("user-%d/", u.ID)
	for rows.Next() {
		var folder string
		if e = rows.Scan(&folder); e != nil {
			s.fail(w, e)
			return
		}
		if strings.HasPrefix(folder, prefix) {
			folders = append(folders, strings.TrimPrefix(folder, prefix))
		}
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	send(w, folders)
}
