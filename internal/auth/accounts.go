package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"gallery/internal/access"
	"gallery/internal/config"
	"net/http"
	"regexp"
	"strings"
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{1,63}$`)

func ValidUsername(s string) bool { return usernamePattern.MatchString(s) }

// Bootstrap runs only for an empty accounts table. Existing credentials are owned
// by the database; restarting never undoes a user's password change.
func Bootstrap(db *sql.DB, c config.Config) error {
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var n int
	if e = tx.QueryRow("SELECT count(*) FROM users").Scan(&n); e != nil {
		return e
	}
	if n != 0 {
		return nil
	}
	if !ValidUsername(c.Username) {
		return fmt.Errorf("bootstrap username must be 2..64 letters, digits, dots, underscores or hyphens")
	}
	result, e := tx.Exec("INSERT INTO users(username,display_name,password_hash,role) VALUES(?,?,?,'admin')", c.Username, c.Username, c.PasswordHash)
	if e != nil {
		return e
	}
	id, e := result.LastInsertId()
	if e != nil {
		return e
	}
	if _, e = tx.Exec("UPDATE albums SET owner_id=? WHERE owner_id IS NULL", id); e != nil {
		return e
	}
	if _, e = tx.Exec("UPDATE album_assets SET added_by=? WHERE added_by IS NULL", id); e != nil {
		return e
	}
	if _, e = tx.Exec("INSERT INTO user_favorites(user_id,asset_id) SELECT ?,id FROM assets WHERE favorite=1", id); e != nil {
		return e
	}
	return tx.Commit()
}
func (a *Auth) InitError() error { return a.initErr }
func (a *Auth) changePassword(w http.ResponseWriter, r *http.Request) {
	if a.c.TrustProxy {
		http.Error(w, "passwords are managed by your identity provider", 400)
		return
	}
	var b struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if json.NewDecoder(r.Body).Decode(&b) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if len(b.NewPassword) < 12 || len(b.NewPassword) > 1024 {
		http.Error(w, "new password must contain 12..1024 bytes", 400)
		return
	}
	select {
	case a.hashSlot <- struct{}{}:
		defer func() { <-a.hashSlot }()
	default:
		http.Error(w, "password service busy", 429)
		return
	}
	u := access.Current(r)
	var hash string
	if e := a.db.QueryRowContext(r.Context(), "SELECT password_hash FROM users WHERE id=? AND enabled=1", u.ID).Scan(&hash); e != nil {
		http.Error(w, "account unavailable", 401)
		return
	}
	if !Verify(hash, b.CurrentPassword) {
		http.Error(w, "current password is incorrect", 403)
		return
	}
	next, e := Hash(b.NewPassword)
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	tx, e := a.db.BeginTx(r.Context(), nil)
	if e != nil {
		http.Error(w, "password update failed", 500)
		return
	}
	defer tx.Rollback()
	result, e := tx.ExecContext(r.Context(), "UPDATE users SET password_hash=?,must_change_password=0 WHERE id=? AND password_hash=?", next, u.ID, hash)
	if e != nil {
		http.Error(w, "password update failed", 500)
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		http.Error(w, "account changed; sign in again", 409)
		return
	}
	if _, e = tx.ExecContext(r.Context(), "DELETE FROM sessions WHERE user_id=?", u.ID); e != nil {
		http.Error(w, "session revocation failed", 500)
		return
	}
	if e = tx.Commit(); e != nil {
		http.Error(w, "password update failed", 500)
		return
	}
	http.SetCookie(w, a.cookie("", -1))
	w.WriteHeader(204)
}
func Lookup(ctx context.Context, db *sql.DB, username string) (access.User, string, error) {
	var u access.User
	var hash string
	e := db.QueryRowContext(ctx, "SELECT id,username,display_name,role,enabled,must_change_password,password_hash FROM users WHERE username=?", strings.TrimSpace(username)).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Enabled, &u.MustChangePassword, &hash)
	return u, hash, e
}
