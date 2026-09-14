package jobs

import (
	"context"
	"database/sql"
	"errors"
	"gallery/internal/config"
	"gallery/internal/database"
	"gallery/internal/library"
	"gallery/internal/scanner"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testRunner struct {
	onCall func(string)
	fail   bool
	calls  atomic.Int64
}

func (r *testRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.calls.Add(1)
	if r.onCall != nil {
		r.onCall(name)
	}
	if r.fail {
		return nil, errors.New("corrupt input")
	}
	if name == "exiftool" {
		return []byte(`[{"ImageWidth":800,"ImageHeight":600,"Model":"SONY"}]`), nil
	}
	if name == "vipsthumbnail" {
		dest := strings.Split(args[len(args)-1], "[")[0]
		return nil, os.WriteFile(dest, []byte("test derivative"), 0600)
	}
	return nil, nil
}
func setup(t *testing.T) (*Pool, *scanner.Scanner) {
	t.Helper()
	root := t.TempDir()
	db, e := database.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	libs := []config.Library{{ID: "test", Name: "Test", Root: root, Enabled: true}}
	if e = library.Sync(context.Background(), db, libs); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(root, "a.jpg"), []byte("source"), 0600)
	s := &scanner.Scanner{DB: db, Libraries: libs}
	if _, e = s.Scan(context.Background()); e != nil {
		t.Fatal(e)
	}
	return &Pool{DB: db, Cache: t.TempDir(), Workers: 1, Runner: &testRunner{}}, s
}
func TestProcessingAndRecovery(t *testing.T) {
	p, _ := setup(t)
	ctx := context.Background()
	j, e := p.claim(ctx)
	if e != nil {
		t.Fatal(e)
	}
	p.process(ctx, j)
	var status string
	if e = p.DB.QueryRow("SELECT preview_status FROM assets").Scan(&status); e != nil || status != "ready" {
		t.Fatal(status, e)
	}
	if _, e = p.claim(ctx); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("completed job remained")
	}
	p.DB.Exec("INSERT INTO jobs(asset_id,state) VALUES(1,'running')")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if e = p.Start(cancelled); e == nil {
		t.Fatal("cancelled recovery succeeded")
	}
	run, stop := context.WithCancel(ctx)
	if e = p.Start(run); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		p.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&n)
		if n == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	p.Wait()
	var n int
	p.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&n)
	if n != 0 {
		t.Fatal("running job not recovered")
	}
}
func TestFailuresAndStaleGeneration(t *testing.T) {
	p, s := setup(t)
	ctx := context.Background()
	p.Runner = &testRunner{fail: true}
	j, e := p.claim(ctx)
	if e != nil {
		t.Fatal(e)
	}
	p.process(ctx, j)
	var state, status string
	p.DB.QueryRow("SELECT state FROM jobs").Scan(&state)
	p.DB.QueryRow("SELECT preview_status FROM assets").Scan(&status)
	if state != "pending" || status != "failed" {
		t.Fatal(state, status)
	}
	p.DB.Exec("UPDATE jobs SET next_at=0,attempts=2")
	j, e = p.claim(ctx)
	if e != nil {
		t.Fatal(e)
	}
	p.process(ctx, j)
	p.DB.QueryRow("SELECT state FROM jobs").Scan(&state)
	if state != "failed" {
		t.Fatal("unbounded retry")
	}
	p.DB.Exec("UPDATE jobs SET state='pending',next_at=0,attempts=0")
	j, e = p.claim(ctx)
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(s.Libraries[0].Root, "a.jpg"), []byte("changed"), 0600)
	if _, e = s.Scan(ctx); e != nil {
		t.Fatal(e)
	}
	p.process(ctx, j)
	p.DB.QueryRow("SELECT state FROM jobs").Scan(&state)
	if state != "pending" {
		t.Fatal("stale worker overwrote queued change")
	}
	p.DB.QueryRow("SELECT preview_status FROM assets").Scan(&status)
	if status != "pending" {
		t.Fatal("stale worker published old result")
	}
}

func TestProcessingPhases(t *testing.T) {
	p, _ := setup(t)
	seen := map[string]bool{}
	p.Runner = &testRunner{onCall: func(tool string) {
		want := "metadata"
		if tool == "vipsthumbnail" {
			want = "previews"
		}
		var stage string
		if err := p.DB.QueryRow("SELECT stage FROM jobs WHERE asset_id=1 AND state='running'").Scan(&stage); err != nil || stage != want {
			t.Fatalf("%s stage: %q, %v", tool, stage, err)
		}
		seen[stage] = true
	}}
	j, err := p.claim(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p.process(context.Background(), j)
	if !seen["metadata"] || !seen["previews"] {
		t.Fatal(seen)
	}
}
