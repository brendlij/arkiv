package main

import (
	"context"
	"gallery/internal/auth"
	"gallery/internal/config"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVersionHasDevelopmentDefault(t *testing.T) {
	if version != "dev" {
		t.Fatalf("unexpected development version %q", version)
	}
}

func TestBootEmbeddedFrontendAndShutdown(t *testing.T) {
	hash, e := auth.Hash("gallery-test-password")
	if e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	base := t.TempDir()
	c := config.Config{Database: filepath.Join(base, "data", "db.sqlite"), Cache: filepath.Join(base, "cache"), Port: "0", Username: "test", PasswordHash: hash, Workers: 1, ScanInterval: time.Minute, Libraries: []config.Library{{ID: "test", Name: "Test", Root: t.TempDir(), Enabled: true}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serve(ctx, c, listener) }()
	client := http.Client{Timeout: 3 * time.Second}
	url := "http://" + listener.Addr().String()
	r, e := client.Get(url + "/api/health")
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	r, e = client.Get(url + "/")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 200 || !strings.Contains(string(b), `<div id="app">`) {
		t.Fatal("embedded frontend missing", string(b))
	}
	r, e = client.Get(url + "/api/assets")
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 401 {
		t.Fatal("API not protected")
	}
	cancel()
	select {
	case e = <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("graceful shutdown timed out")
	}
}
