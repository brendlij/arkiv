package auth

import (
	"encoding/json"
	"gallery/internal/access"
	"net/http"
	"strconv"
	"strings"
)

func (a *Auth) manageUsers(w http.ResponseWriter, r *http.Request) {
	if !access.Admin(w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/users")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if path == "" && r.Method == "GET" {
		rows, e := a.db.QueryContext(r.Context(), "SELECT id,username,display_name,role,enabled,must_change_password FROM users ORDER BY username LIMIT 1001")
		if e != nil {
			http.Error(w, "accounts unavailable", 500)
			return
		}
		defer rows.Close()
		items := []access.User{}
		for rows.Next() {
			var u access.User
			if e = rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Enabled, &u.MustChangePassword); e != nil {
				http.Error(w, "accounts unavailable", 500)
				return
			}
			items = append(items, u)
		}
		if rows.Err() != nil {
			http.Error(w, "accounts unavailable", 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items})
		return
	}
	var b struct {
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
		Enabled     *bool  `json:"enabled"`
		Password    string `json:"password"`
	}
	if json.NewDecoder(r.Body).Decode(&b) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	b.Username = strings.TrimSpace(b.Username)
	b.DisplayName = strings.TrimSpace(b.DisplayName)
	create := path == "" && r.Method == "POST"
	reset := len(parts) == 2 && parts[1] == "password" && r.Method == "POST"
	id, _ := strconv.ParseInt(parts[0], 10, 64)
	if !create && !reset && !(len(parts) == 1 && id > 0 && r.Method == "PATCH") {
		http.NotFound(w, r)
		return
	}
	if !reset && (b.Role != "admin" && b.Role != "member" || len([]rune(b.DisplayName)) < 1 || len([]rune(b.DisplayName)) > 120) {
		http.Error(w, "display name and valid role required", 400)
		return
	}
	if create && !ValidUsername(b.Username) {
		http.Error(w, "username must be 2..64 letters, digits, dots, underscores or hyphens", 400)
		return
	}
	var hash string
	if create || reset {
		select {
		case a.hashSlot <- struct{}{}:
			defer func() { <-a.hashSlot }()
		default:
			http.Error(w, "password service busy", 429)
			return
		}
		var e error
		hash, e = Hash(b.Password)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
	}
	tx, e := a.db.BeginTx(r.Context(), nil)
	if e != nil {
		http.Error(w, "account update failed", 500)
		return
	}
	defer tx.Rollback()
	if create {
		var n int
		if e = tx.QueryRowContext(r.Context(), "SELECT count(*) FROM users").Scan(&n); e != nil {
			http.Error(w, "account lookup failed", 500)
			return
		}
		if n >= 1000 {
			http.Error(w, "account limit reached", 409)
			return
		}
		res, err := tx.ExecContext(r.Context(), "INSERT INTO users(username,display_name,password_hash,role,must_change_password) VALUES(?,?,?,?,1)", b.Username, b.DisplayName, hash, b.Role)
		if err != nil {
			http.Error(w, "username unavailable", 409)
			return
		}
		id, e = res.LastInsertId()
	} else {
		var role string
		var enabled bool
		if e = tx.QueryRowContext(r.Context(), "SELECT role,enabled FROM users WHERE id=?", id).Scan(&role, &enabled); e != nil {
			http.NotFound(w, r)
			return
		}
		if reset {
			if id == access.Current(r).ID {
				http.Error(w, "use your account password form", 400)
				return
			}
			_, e = tx.ExecContext(r.Context(), "UPDATE users SET password_hash=?,must_change_password=1 WHERE id=?", hash, id)
		} else {
			if b.Enabled == nil {
				http.Error(w, "enabled required", 400)
				return
			}
			if id == access.Current(r).ID && (!*b.Enabled || b.Role != "admin") {
				http.Error(w, "you cannot disable or demote your own account", 409)
				return
			}
			if role == "admin" && enabled && (!*b.Enabled || b.Role != "admin") {
				var n int
				if e = tx.QueryRowContext(r.Context(), "SELECT count(*) FROM users WHERE role='admin' AND enabled=1").Scan(&n); e != nil {
					http.Error(w, "account lookup failed", 500)
					return
				}
				if n <= 1 {
					http.Error(w, "at least one active administrator is required", 409)
					return
				}
			}
			_, e = tx.ExecContext(r.Context(), "UPDATE users SET display_name=?,role=?,enabled=? WHERE id=?", b.DisplayName, b.Role, *b.Enabled, id)
		}
		if e == nil {
			_, e = tx.ExecContext(r.Context(), "DELETE FROM sessions WHERE user_id=?", id)
		}
	}
	if e != nil {
		http.Error(w, "account update failed", 500)
		return
	}
	if e = tx.Commit(); e != nil {
		http.Error(w, "account update failed", 500)
		return
	}
	if create {
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{"id": id})
	} else {
		w.WriteHeader(204)
	}
}
