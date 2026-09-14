// Package access is the single source of truth for media and collection access.
package access

import (
	"context"
	"fmt"
	"net/http"
)

type User struct {
	PublicAlbum        int64  `json:"-"`
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	DisplayName        string `json:"displayName"`
	Role               string `json:"role"`
	Enabled            bool   `json:"enabled"`
	MustChangePassword bool   `json:"mustChangePassword"`
}
type contextKey struct{}

func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, contextKey{}, u)
}
func Current(r *http.Request) User { u, _ := r.Context().Value(contextKey{}).(User); return u }
func Admin(w http.ResponseWriter, r *http.Request) bool {
	u := Current(r)
	if u.ID < 1 || u.Role != "admin" {
		http.Error(w, "administrator access required", 403)
		return false
	}
	return true
}

// All interpolation below is restricted to typed numeric IDs and internal SQL aliases.
func Direct(u User, a string) string {
	if u.ID < 1 {
		return "0"
	}
	if u.Role == "admin" {
		return a + ".trashed_at=0"
	}
	return "(" + a + ".trashed_at=0 AND " + grant(fmt.Sprint(u.ID), a) + ")"
}
func grant(id, a string) string {
	return `EXISTS(SELECT 1 FROM folder_grants fg WHERE fg.user_id=` + id + ` AND fg.library_id=` + a + `.library_id AND (fg.folder='' OR ` + a + `.folder=fg.folder OR substr(` + a + `.folder,1,length(fg.folder)+1)=fg.folder||'/'))`
}
func Album(u User, al string) string {
	if u.PublicAlbum > 0 {
		return fmt.Sprintf("%s.id=%d", al, u.PublicAlbum)
	}
	if u.ID < 1 {
		return "0"
	}
	if u.Role == "admin" {
		return "1"
	}
	id := fmt.Sprint(u.ID)
	return `(` + al + `.owner_id=` + id + ` OR EXISTS(SELECT 1 FROM album_members am WHERE am.album_id=` + al + `.id AND am.user_id=` + id + ` AND am.starts_at<=unixepoch() AND (am.expires_at=0 OR am.expires_at>unixepoch())))`
}
func visibleBase(u User, a string) string {
	direct := Direct(u, a)
	if direct == "1" || (direct == "0" && u.PublicAlbum == 0) {
		return direct
	}
	return `(` + a + `.trashed_at=0 AND (` + direct + ` OR EXISTS(SELECT 1 FROM album_assets shared JOIN albums al ON al.id=shared.album_id JOIN users owner ON owner.id=al.owner_id AND owner.enabled=1 JOIN users publisher ON publisher.id=shared.added_by AND publisher.enabled=1 WHERE shared.asset_id=` + a + `.id AND ` + Album(u, "al") + ` AND (publisher.role='admin' OR ` + grant("publisher.id", a) + `))))`
}

// Personal removals never alter another user's access or public albums.
func Visible(u User, a string) string {
	base := "(" + visibleBase(u, a) + " AND NOT EXISTS(SELECT 1 FROM media_deletions md WHERE md.asset_id=" + a + ".id))"
	if u.ID > 0 && u.PublicAlbum == 0 {
		base = "(" + base + fmt.Sprintf(" AND NOT EXISTS(SELECT 1 FROM personal_trash pt WHERE pt.user_id=%d AND pt.asset_id=%s.id))", u.ID, a)
	}
	return base
}
func Trash(u User, a string) string {
	if u.ID < 1 || u.PublicAlbum != 0 {
		return "0"
	}
	return "((" + a + ".trashed_at>0 AND " + Manage(u, a) + ") OR (" + visibleBase(u, a) + fmt.Sprintf(" AND EXISTS(SELECT 1 FROM personal_trash pt WHERE pt.user_id=%d AND pt.asset_id=%s.id AND pt.dismissed=0)))", u.ID, a) + " AND NOT EXISTS(SELECT 1 FROM media_deletions md WHERE md.asset_id=" + a + ".id)"
}
func Favorite(u User) string {
	return fmt.Sprintf("EXISTS(SELECT 1 FROM user_favorites uf WHERE uf.user_id=%d AND uf.asset_id=a.id)", u.ID)
}

// Managed originals retain their immutable per-user storage prefix when organized.
func Manage(u User, a string) string {
	if u.ID < 1 || u.PublicAlbum != 0 {
		return "0"
	}
	return "NOT EXISTS(SELECT 1 FROM media_deletions md WHERE md.asset_id=" + a + ".id) AND " + fmt.Sprintf("(%s.library_id='arkiv-uploads' AND substr(%s.relative_path,1,length('user-%d/'))='user-%d/')", a, a, u.ID, u.ID)
}
func ReadFile(u User, a string) string {
	return "(" + Visible(u, a) + " OR " + Manage(u, a) + " OR " + Trash(u, a) + ")"
}
