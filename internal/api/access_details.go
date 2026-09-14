package api

import (
	"fmt"
	"gallery/internal/access"
	"net/http"
	"strconv"
	"strings"
)

// Access explanations use the same predicates as media serving. Only owners
// and administrators receive other users' identities.
func (s *Server) accessDetails(w http.ResponseWriter, r *http.Request) {
	u := access.Current(r)
	if u.ID < 1 || u.PublicAlbum != 0 {
		http.Error(w, "Sign in required", 401)
		return
	}
	lib, folder := r.URL.Query().Get("library"), r.URL.Query().Get("folder")
	assetID := int64(0)
	ownerID := int64(0)
	trashed := false
	scope := "a.library_id=? AND (a.folder=? OR substr(a.folder,1,length(?)+1)=?||'/' OR ?='')"
	args := []any{lib, folder, folder, folder, folder}
	if raw := r.URL.Query().Get("asset"); raw != "" {
		var e error
		assetID, e = strconv.ParseInt(raw, 10, 64)
		if e != nil || assetID < 1 {
			http.Error(w, "Invalid asset", 400)
			return
		}
		var rel string
		var deleted int64
		e = s.DB.QueryRowContext(r.Context(), "SELECT a.library_id,a.folder,a.relative_path,a.trashed_at FROM assets a JOIN libraries l ON l.id=a.library_id WHERE l.enabled=1 AND a.id=? AND "+access.ReadFile(u, "a"), assetID).Scan(&lib, &folder, &rel, &deleted)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		trashed = deleted > 0
		if lib == "arkiv-uploads" {
			ownerID, _ = strconv.ParseInt(strings.TrimPrefix(strings.Split(rel, "/")[0], "user-"), 10, 64)
		}
		scope = "a.id=?"
		args = []any{assetID}
	} else {
		if lib == "" {
			http.Error(w, "Choose a library", 400)
			return
		}
		var n int
		e := s.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM assets a JOIN libraries l ON l.id=a.library_id WHERE l.enabled=1 AND ("+scope+") AND "+access.Direct(u, "a"), args...).Scan(&n)
		ownRoot := fmt.Sprintf("user-%d", u.ID)
		own := lib == "arkiv-uploads" && (folder == ownRoot || strings.HasPrefix(folder, ownRoot+"/"))
		if e != nil {
			s.fail(w, e)
			return
		}
		if n == 0 && lib == "arkiv-uploads" {
			e = s.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM managed_folders f WHERE f.path=? AND EXISTS(SELECT 1 FROM folder_grants fg WHERE fg.user_id=? AND fg.library_id='arkiv-uploads' AND (fg.folder='' OR f.path=fg.folder OR substr(f.path,1,length(fg.folder)+1)=fg.folder||'/'))", folder, u.ID).Scan(&n)
			if e != nil {
				s.fail(w, e)
				return
			}
		}
		if n == 0 && !own && u.Role != "admin" {
			http.NotFound(w, r)
			return
		}
		if lib == "arkiv-uploads" {
			ownerID, _ = strconv.ParseInt(strings.TrimPrefix(strings.Split(folder, "/")[0], "user-"), 10, 64)
		}
	}
	owner := "Server library (managed by administrators)"
	if lib == "arkiv-uploads" {
		owner = "Multiple upload owners"
	}
	if ownerID > 0 {
		if e := s.DB.QueryRowContext(r.Context(), "SELECT display_name FROM users WHERE id=?", ownerID).Scan(&owner); e != nil {
			s.fail(w, e)
			return
		}
		if ownerID == u.ID {
			owner += " (you)"
		}
	}
	detailed := u.Role == "admin" || ownerID == u.ID
	type person struct {
		Name   string `json:"name"`
		Access string `json:"access"`
	}
	people := []person{}
	if detailed {
		rows, e := s.DB.QueryContext(r.Context(), "SELECT id,display_name,role FROM users WHERE enabled=1 ORDER BY display_name")
		if e != nil {
			s.fail(w, e)
			return
		}
		users := []access.User{}
		for rows.Next() {
			var other access.User
			if e = rows.Scan(&other.ID, &other.DisplayName, &other.Role); e != nil {
				rows.Close()
				s.fail(w, e)
				return
			}
			users = append(users, other)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			s.fail(w, e)
			return
		}
		for _, other := range users {
			label := "Can view items here through folder or album access"
			predicate := access.Visible(other, "a")
			if assetID > 0 {
				predicate = access.Visible(other, "a")
				label = "Can view and download"
			}
			var n int
			if e = s.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM assets a WHERE ("+scope+") AND "+predicate, args...).Scan(&n); e != nil {
				s.fail(w, e)
				return
			}
			if assetID == 0 && lib == "arkiv-uploads" && !trashed {
				var grants int
				if e = s.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM folder_grants WHERE user_id=? AND library_id=? AND (folder='' OR folder=? OR substr(?,1,length(folder)+1)=folder||'/' OR substr(folder,1,length(?)+1)=?||'/')", other.ID, lib, folder, folder, folder, folder).Scan(&grants); e != nil {
					s.fail(w, e)
					return
				}
				if grants > 0 {
					n = 1
				}
			}
			if other.Role == "admin" && !trashed {
				label = "Administrator · can view all non-trashed media"
				n = 1
			}
			if other.ID == ownerID {
				label = "Owner · can organize and trash uploads"
				n = 1
			}
			if n > 0 {
				if other.ID == u.ID {
					other.DisplayName += " (you)"
				}
				people = append(people, person{other.DisplayName, label})
			}
		}
	}
	publicAlbums := []string{}
	if detailed && !trashed {
		rows, e := s.DB.QueryContext(r.Context(), "SELECT al.id,al.name FROM albums al JOIN album_public_shares ps ON ps.album_id=al.id WHERE ps.starts_at<=unixepoch() AND (ps.expires_at=0 OR ps.expires_at>unixepoch())")
		if e != nil {
			s.fail(w, e)
			return
		}
		type album struct {
			id   int64
			name string
		}
		albums := []album{}
		for rows.Next() {
			var a album
			if e = rows.Scan(&a.id, &a.name); e != nil {
				rows.Close()
				s.fail(w, e)
				return
			}
			albums = append(albums, a)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			s.fail(w, e)
			return
		}
		for _, a := range albums {
			var n int
			if e = s.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM assets a WHERE ("+scope+") AND "+access.Visible(access.User{PublicAlbum: a.id}, "a"), args...).Scan(&n); e != nil {
				s.fail(w, e)
				return
			}
			if n > 0 {
				publicAlbums = append(publicAlbums, a.name)
			}
		}
	}
	note := "Folder access includes subfolders and future additions. People listed may have access to only part of this folder. Album access covers only included items. Scheduled shares may grant access later."
	if assetID > 0 {
		note = "Access shown is effective now. Scheduled shares may grant access later; check the album's sharing settings. Downloaded copies cannot be recalled."
	}
	if !detailed {
		note = "You can view this content. Only its owner or an administrator can inspect the full list of people with access. Administrators can view all non-trashed media."
	}
	if trashed {
		note = "In Trash: only the uploader can access or restore this item."
	}
	storage := "Read-only server files. Organize the original folders outside arkiv."
	if lib == "arkiv-uploads" {
		storage = "Uploaded media. Folders organize files inside arkiv without changing the original storage path."
	}
	send(w, map[string]any{"owner": owner, "storage": storage, "people": people, "publicAlbums": publicAlbums, "detailed": detailed, "note": note, "canManage": ownerID == u.ID})
}
