package main

import (
	"bufio"
	"context"
	"fmt"
	"gallery/internal/api"
	"gallery/internal/auth"
	"gallery/internal/config"
	"gallery/internal/database"
	"gallery/internal/jobs"
	"gallery/internal/library"
	"gallery/internal/metadata"
	"gallery/internal/places"
	"gallery/internal/scanner"
	"gallery/web"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		line, e := bufio.NewReader(os.Stdin).ReadString('\n')
		if e != nil && len(line) == 0 {
			fmt.Fprintln(os.Stderr, "provide password on stdin")
			os.Exit(1)
		}
		hash, e := auth.Hash(strings.TrimRight(line, "\r\n"))
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		fmt.Println(hash)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		c := http.Client{Timeout: 3 * time.Second}
		port := config.HTTPPort()
		r, e := c.Get("http://127.0.0.1:" + port + "/api/health")
		if e != nil {
			os.Exit(1)
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	if e := run(); e != nil {
		slog.Error("application stopped", "error", e)
		os.Exit(1)
	}
}
func run() error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	c, e := config.Load()
	if e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx, c, nil)
}
func serve(parent context.Context, c config.Config, listener net.Listener) error {
	ctx, stop := context.WithCancel(parent)
	defer stop()
	var e error
	if !c.TrustProxy {
		if e = auth.ValidateHash(c.PasswordHash); e != nil {
			return e
		}
	}
	if e = os.MkdirAll(c.Cache, 0700); e != nil {
		return e
	}
	if c.Uploads != "" {
		if e = os.MkdirAll(c.Uploads, 0700); e != nil {
			return e
		}
	}
	db, e := database.Open(c.Database)
	if e != nil {
		return e
	}
	defer db.Close()
	if e = library.Sync(ctx, db, c.Libraries); e != nil {
		return e
	}
	if e = places.Backfill(ctx, db); e != nil {
		return e
	}
	pool := &jobs.Pool{DB: db, Cache: c.Cache, Workers: c.Workers, Runner: metadata.Command{}}
	if e = pool.Start(ctx); e != nil {
		return e
	}
	scanLibraries := []config.Library{}
	for _, l := range c.Libraries {
		if l.ID != "arkiv-uploads" {
			scanLibraries = append(scanLibraries, l)
		}
	}
	scan := &scanner.Scanner{DB: db, Libraries: scanLibraries}
	requests := make(chan struct{}, 1)
	requestScan := func() bool {
		if scan.Running.Load() {
			return false
		}
		select {
		case requests <- struct{}{}:
			return true
		default:
			return false
		}
	}
	var scanning sync.WaitGroup
	scanning.Add(1)
	go func() {
		defer scanning.Done()
		ticker := time.NewTicker(c.ScanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-requests:
			}
			if _, err := scan.Scan(ctx); err != nil && ctx.Err() == nil {
				slog.Error("scan incomplete", "error", err)
			}
		}
	}()
	defer func() { stop(); scanning.Wait(); pool.Wait() }()
	if c.ScanOnStart {
		requestScan()
	}
	mux := http.NewServeMux()
	apiServer := &api.Server{DB: db, Cache: c.Cache, Uploads: c.Uploads, UploadQuota: c.UploadQuota, UploadRetention: c.UploadRetention, Scanner: scan, RequestScan: requestScan}
	apiServer.Register(mux)
	maintenanceDone := make(chan struct{})
	go func() {
		defer close(maintenanceDone)
		var tasks sync.WaitGroup
		tasks.Add(3)
		go func() { defer tasks.Done(); apiServer.MaintainUploads(ctx) }()
		go func() { defer tasks.Done(); apiServer.MaintainCache(ctx) }()
		go func() { defer tasks.Done(); apiServer.RunVideoJobs(ctx) }()
		tasks.Wait()
	}()
	defer func() { stop(); <-maintenanceDone }()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if e := db.PingContext(r.Context()); e != nil {
			http.Error(w, "unhealthy", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	mux.Handle("/", web.Handler())
	authentication := auth.New(db, c)
	if e := authentication.InitError(); e != nil {
		return e
	}
	server := &http.Server{Addr: ":" + c.Port, Handler: authentication.Handler(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	done := make(chan error, 1)
	if listener == nil {
		listener, e = net.Listen("tcp", server.Addr)
		if e != nil {
			return e
		}
	}
	go func() {
		slog.Info("server listening", "address", listener.Addr().String())
		done <- server.Serve(listener)
	}()
	select {
	case e = <-done:
		if e != http.ErrServerClosed {
			return e
		}
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}
