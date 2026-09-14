package config

import (
	"path/filepath"
	"testing"
)

func TestValidation(t *testing.T) {
	base := t.TempDir()
	t.Setenv("ARKIV_DATABASE", filepath.Join(base, "data", "gallery.db"))
	t.Setenv("ARKIV_CACHE", filepath.Join(base, "cache"))
	t.Setenv("ARKIV_LIBRARY", filepath.Join(base, "photos"))
	t.Setenv("ARKIV_LIBRARIES", "")
	t.Setenv("ARKIV_PASSWORD_HASH", "test")
	t.Setenv("ARKIV_TRUST_PROXY_AUTH", "false")
	if _, e := Load(); e != nil {
		t.Fatal(e)
	}
	t.Setenv("ARKIV_UPLOAD_QUOTA_BYTES", "0")
	if _, e := Load(); e == nil {
		t.Fatal("zero quota accepted")
	}
	t.Setenv("ARKIV_UPLOAD_QUOTA_BYTES", "10737418240")
	t.Setenv("ARKIV_UPLOAD_RETENTION", "10m")
	if _, e := Load(); e == nil {
		t.Fatal("unsafe retention accepted")
	}
	t.Setenv("ARKIV_UPLOAD_RETENTION", "24h")
	if c, e := Load(); e != nil || c.UploadQuota != 10737418240 {
		t.Fatalf("quota configuration %v", e)
	}
	t.Setenv("ARKIV_WORKERS", "999")
	if _, e := Load(); e == nil {
		t.Fatal("invalid workers")
	}
	t.Setenv("ARKIV_WORKERS", "2")
	t.Setenv("ARKIV_SCAN_INTERVAL", "1s")
	if _, e := Load(); e == nil {
		t.Fatal("invalid scan interval")
	}
	t.Setenv("ARKIV_SCAN_INTERVAL", "30m")
	t.Setenv("ARKIV_CACHE", filepath.Join(base, "photos", "cache"))
	if _, e := Load(); e == nil {
		t.Fatal("cache within library")
	}
}

func TestArkivEnvironment(t *testing.T) {
	base := t.TempDir()
	t.Setenv("ARKIV_CACHE", filepath.Join(base, "cache"))
	t.Setenv("ARKIV_LIBRARY", filepath.Join(base, "photos"))
	t.Setenv("ARKIV_PASSWORD_HASH", "arkiv-hash")
	t.Setenv("ARKIV_PORT", "8096")
	t.Setenv("ARKIV_DATABASE", filepath.Join(base, "data", "gallery.db"))
	t.Setenv("ARKIV_UPLOAD_RETENTION", "24h")
	current, e := Load()
	if e != nil {
		t.Fatal(e)
	}
	if current.PasswordHash != "arkiv-hash" || HTTPPort() != "8096" || current.UploadRetention.Hours() != 24 {
		t.Fatal(current)
	}
	t.Setenv("ARKIV_PASSWORD_HASH", "")
	if _, e = Load(); e == nil {
		t.Fatal("empty password hash accepted")
	}
}
