package api

import (
	"encoding/json"
	"gallery/internal/access"
	"net/http"
	"strconv"
	"time"
)

// Processing details contain server paths and errors, so are administrator-only.
func (s *Server) processing(w http.ResponseWriter, r *http.Request) {
	if !access.Admin(w, r) {
		return
	}
	filter := r.URL.Query().Get("state")
	if filter == "" {
		filter = "failed"
	}
	if filter != "failed" && filter != "pending" && filter != "running" {
		http.Error(w, "invalid state", 400)
		return
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		var err error
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > 10000000 {
			http.Error(w, "invalid offset", 400)
			return
		}
	}
	ctx := r.Context()
	// A read transaction keeps the counts and queue page consistent.
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback()
	var total, metadataReady, previewsReady int64
	err = tx.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(metadata_status='ready'),0),coalesce(sum(preview_status='ready'),0) FROM assets a JOIN libraries l ON l.id=a.library_id WHERE l.enabled=1`).Scan(&total, &metadataReady, &previewsReady)
	if err != nil {
		s.fail(w, err)
		return
	}
	counts := map[string]int64{"pending": 0, "running": 0, "failed": 0}
	rows, err := tx.QueryContext(ctx, `SELECT j.state,count(*) FROM jobs j JOIN assets a ON a.id=j.asset_id JOIN libraries l ON l.id=a.library_id WHERE l.enabled=1 GROUP BY j.state`)
	if err != nil {
		s.fail(w, err)
		return
	}
	for rows.Next() {
		var state string
		var n int64
		if err = rows.Scan(&state, &n); err != nil {
			break
		}
		counts[state] = n
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		s.fail(w, err)
		return
	}
	type item struct {
		ID       int64  `json:"id"`
		Filename string `json:"filename"`
		Folder   string `json:"folder"`
		Library  string `json:"library"`
		Stage    string `json:"stage"`
		Metadata string `json:"metadata"`
		Preview  string `json:"preview"`
		Error    string `json:"error"`
		Attempts int    `json:"attempts"`
		NextAt   int64  `json:"nextAt"`
		Trashed  bool   `json:"trashed"`
	}
	items := []item{}
	rows, err = tx.QueryContext(ctx, `SELECT a.id,a.filename,a.folder,l.name,j.stage,a.metadata_status,a.preview_status,a.error,j.attempts,j.next_at,a.trashed_at<>0 FROM jobs j JOIN assets a ON a.id=j.asset_id JOIN libraries l ON l.id=a.library_id WHERE l.enabled=1 AND j.state=? ORDER BY a.id LIMIT 50 OFFSET ?`, filter, offset)
	if err != nil {
		s.fail(w, err)
		return
	}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.ID, &v.Filename, &v.Folder, &v.Library, &v.Stage, &v.Metadata, &v.Preview, &v.Error, &v.Attempts, &v.NextAt, &v.Trashed); err != nil {
			break
		}
		items = append(items, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		s.fail(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		s.fail(w, err)
		return
	}
	lastScan, scanError := "", ""
	running := false
	var examined int64
	if s.Scanner != nil {
		lastScan, scanError = s.Scanner.Status()
		running = s.Scanner.Running.Load()
		examined = s.Scanner.Examined.Load()
	}
	send(w, map[string]any{"total": total, "metadataReady": metadataReady, "previewsReady": previewsReady, "counts": counts, "items": items, "scanRunning": running, "scanExamined": examined, "lastScan": lastScan, "scanError": scanError, "serverTime": time.Now().Unix()})
}

// Retry only terminal failures; healthy and currently running jobs stay untouched.
func (s *Server) retryProcessing(w http.ResponseWriter, r *http.Request) {
	if !access.Admin(w, r) {
		return
	}
	var body struct {
		ID int64 `json:"id"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.ID < 0 {
		http.Error(w, "valid asset id required (0 for all failures)", 400)
		return
	}
	result, err := s.DB.ExecContext(r.Context(), `UPDATE jobs SET state='pending',stage='',attempts=0,next_at=0 WHERE state='failed' AND (?=0 OR asset_id=?) AND asset_id IN (SELECT a.id FROM assets a JOIN libraries l ON l.id=a.library_id WHERE l.enabled=1)`, body.ID, body.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	n, err := result.RowsAffected()
	if err != nil {
		s.fail(w, err)
		return
	}
	send(w, map[string]any{"retried": n})
}
