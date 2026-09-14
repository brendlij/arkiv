package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Library struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Root    string `json:"root"`
	Enabled bool   `json:"enabled"`
}
type Config struct {
	Database, Cache, Port, Username, PasswordHash string
	Libraries                                     []Library
	Uploads                                       string
	UploadQuota                                   int64
	UploadRetention                               time.Duration
	ScanOnStart, SecureCookies, TrustProxy        bool
	ScanInterval                                  time.Duration
	Workers                                       int
	TrustedProxies                                []*net.IPNet
}

func Load() (Config, error) {
	c := Config{Database: env("ARKIV_DATABASE", "/data/gallery.db"), Cache: env("ARKIV_CACHE", "/cache"), Port: HTTPPort(), Username: env("ARKIV_USERNAME", "admin"), PasswordHash: env("ARKIV_PASSWORD_HASH", "")}
	var err error
	c.UploadQuota, err = strconv.ParseInt(env("ARKIV_UPLOAD_QUOTA_BYTES", "5368709120"), 10, 64)
	if err != nil || c.UploadQuota < 1 || c.UploadQuota > 1<<50 {
		return c, fmt.Errorf("ARKIV_UPLOAD_QUOTA_BYTES must be 1..1125899906842624")
	}
	c.UploadRetention, err = time.ParseDuration(env("ARKIV_UPLOAD_RETENTION", "168h"))
	if err != nil || c.UploadRetention < time.Hour || c.UploadRetention > 90*24*time.Hour {
		return c, fmt.Errorf("ARKIV_UPLOAD_RETENTION must be between 1h and 2160h")
	}

	for key, dst := range map[string]*bool{"ARKIV_SCAN_ON_START": &c.ScanOnStart, "ARKIV_SECURE_COOKIES": &c.SecureCookies, "ARKIV_TRUST_PROXY_AUTH": &c.TrustProxy} {
		def := "false"
		if key == "ARKIV_SCAN_ON_START" {
			def = "true"
		}
		*dst, err = strconv.ParseBool(env(key, def))
		if err != nil {
			return c, fmt.Errorf("%s: %w", key, err)
		}
	}
	c.Workers, err = strconv.Atoi(env("ARKIV_WORKERS", "2"))
	if err != nil || c.Workers < 1 || c.Workers > 16 {
		return c, fmt.Errorf("ARKIV_WORKERS must be 1..16")
	}
	c.ScanInterval, err = time.ParseDuration(env("ARKIV_SCAN_INTERVAL", "30m"))
	if err != nil || c.ScanInterval < time.Minute {
		return c, fmt.Errorf("ARKIV_SCAN_INTERVAL must be at least 1m")
	}
	p, e := strconv.Atoi(c.Port)
	if e != nil || p < 1 || p > 65535 {
		return c, fmt.Errorf("invalid ARKIV_PORT")
	}
	if raw := env("ARKIV_LIBRARIES", ""); raw != "" {
		if err = json.Unmarshal([]byte(raw), &c.Libraries); err != nil {
			return c, fmt.Errorf("ARKIV_LIBRARIES: %w", err)
		}
	} else {
		c.Libraries = []Library{{ID: "photos", Name: "Photos", Root: env("ARKIV_LIBRARY", "/photos"), Enabled: true}}
	}
	c.Uploads = filepath.Join(filepath.Dir(c.Database), "uploads")
	c.Libraries = append(c.Libraries, Library{ID: "arkiv-uploads", Name: "Uploads", Root: c.Uploads, Enabled: true})
	seen := map[string]bool{}
	for i := range c.Libraries {
		l := &c.Libraries[i]
		if l.ID == "" || seen[l.ID] || !filepath.IsAbs(l.Root) {
			return c, fmt.Errorf("library requires unique ID and absolute root")
		}
		seen[l.ID] = true
		l.Root = filepath.Clean(l.Root)
	}
	if !filepath.IsAbs(c.Database) || !filepath.IsAbs(c.Cache) {
		return c, fmt.Errorf("database and cache paths must be absolute")
	}
	for _, path := range []string{c.Database, c.Cache, c.Uploads} {
		if err := rejectSymlinkParents(path); err != nil {
			return c, err
		}
	}
	for _, l := range c.Libraries {
		for _, p := range []string{c.Database, c.Cache} {
			r, e := filepath.Rel(l.Root, p)
			if e == nil && r != ".." && !hasParent(r) {
				return c, fmt.Errorf("application data must be outside libraries")
			}
		}
		if rel, e := filepath.Rel(c.Cache, l.Root); e == nil && filepath.IsLocal(rel) {
			return c, fmt.Errorf("library must not be inside cache")
		}
	}
	if len(c.Libraries) == 0 {
		return c, fmt.Errorf("at least one library must be configured")
	}
	if strings.TrimSpace(c.Username) == "" || len(c.Username) > 128 {
		return c, fmt.Errorf("invalid username")
	}
	for i, l := range c.Libraries {
		for _, other := range c.Libraries[i+1:] {
			if rel, e := filepath.Rel(l.Root, other.Root); e == nil && filepath.IsLocal(rel) {
				return c, fmt.Errorf("library roots must not overlap")
			}
			if rel, e := filepath.Rel(other.Root, l.Root); e == nil && filepath.IsLocal(rel) {
				return c, fmt.Errorf("library roots must not overlap")
			}
		}
	}
	if c.TrustProxy {
		var cidrs []string
		if err = json.Unmarshal([]byte(env("ARKIV_TRUSTED_PROXIES", "")), &cidrs); err != nil || len(cidrs) == 0 {
			return c, fmt.Errorf("proxy auth requires ARKIV_TRUSTED_PROXIES JSON CIDRs")
		}
		for _, s := range cidrs {
			_, n, e := net.ParseCIDR(s)
			if e != nil {
				return c, e
			}
			c.TrustedProxies = append(c.TrustedProxies, n)
		}
	} else if c.PasswordHash == "" {
		return c, fmt.Errorf("ARKIV_PASSWORD_HASH is required; run arkiv hash-password")
	}
	return c, nil
}
func hasParent(s string) bool { return len(s) > 3 && s[:3] == ".."+string(filepath.Separator) }

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// HTTPPort is also used by the standalone container health check.
func HTTPPort() string { return env("ARKIV_PORT", "8090") }

func rejectSymlinkParents(path string) error {
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("application storage must not use symlinks: %s", p)
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}
