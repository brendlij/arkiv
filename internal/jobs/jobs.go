package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"gallery/internal/library"
	"gallery/internal/media"
	"gallery/internal/metadata"
	"gallery/internal/places"
	"gallery/internal/thumbnails"
	"gallery/internal/video"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Pool struct {
	DB      *sql.DB
	Cache   string
	Workers int
	Runner  metadata.Runner
	wg      sync.WaitGroup
}
type job struct {
	ID, Generation, Size, MTime int64
	Root, Relative, Kind        string
	Attempts                    int
}

func (p *Pool) Start(ctx context.Context) error {
	// Only app-owned temporary directories are cleaned; originals are never here.
	entries, e := os.ReadDir(p.Cache)
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "processing-") {
			if e = os.RemoveAll(filepath.Join(p.Cache, entry.Name())); e != nil {
				return fmt.Errorf("clean interrupted processing: %w", e)
			}
		}
	}
	if _, e := p.DB.ExecContext(ctx, "UPDATE jobs SET state='pending',stage='' WHERE state='running'"); e != nil {
		return e
	}
	for i := 0; i < p.Workers; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			timer := time.NewTicker(2 * time.Second)
			defer timer.Stop()
			for {
				if ctx.Err() != nil {
					return
				}
				j, e := p.claim(ctx)
				if e == nil {
					p.process(ctx, j)
					continue
				}
				if !errors.Is(e, sql.ErrNoRows) && ctx.Err() == nil {
					slog.Error("queue claim failed", "error", e)
				}
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
				}
			}
		}()
	}
	return nil
}
func (p *Pool) Wait() { p.wg.Wait() }
func (p *Pool) claim(ctx context.Context) (job, error) {
	j := job{}
	tx, e := p.DB.BeginTx(ctx, nil)
	if e != nil {
		return j, e
	}
	defer tx.Rollback()
	e = tx.QueryRowContext(ctx, `SELECT a.id,a.generation,a.file_size,a.modified_at,l.root,a.relative_path,a.media_type,j.attempts FROM jobs j JOIN assets a ON a.id=j.asset_id JOIN libraries l ON l.id=a.library_id WHERE l.enabled=1 AND NOT EXISTS(SELECT 1 FROM media_deletions md WHERE md.asset_id=a.id) AND j.state='pending' AND j.next_at<=? ORDER BY j.next_at,a.id LIMIT 1`, time.Now().Unix()).Scan(&j.ID, &j.Generation, &j.Size, &j.MTime, &j.Root, &j.Relative, &j.Kind, &j.Attempts)
	if e != nil {
		return j, e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE jobs SET state='running',stage='metadata' WHERE asset_id=?", j.ID); e != nil {
		return j, e
	}
	return j, tx.Commit()
}
func (p *Pool) process(ctx context.Context, j job) {
	source, e := library.Resolve(j.Root, j.Relative)
	d := metadata.Data{TakenAt: time.Unix(0, j.MTime).UTC().Format(time.RFC3339), Orientation: 1}
	var metaErr error
	key := thumbnails.Key(j.ID, j.Generation, j.Size, j.MTime)
	if e == nil {
		var info os.FileInfo
		info, e = os.Stat(source)
		if e == nil && (info.Size() != j.Size || info.ModTime().UnixNano() != j.MTime) {
			e = fmt.Errorf("source changed after scan")
		}
	}
	if e != nil {
		metaErr = e
	}
	if e == nil {
		if j.Kind == "video" {
			d, metaErr = video.Probe(ctx, p.Runner, source, time.Unix(0, j.MTime))
		} else {
			d, metaErr = metadata.Extract(ctx, p.Runner, source, time.Unix(0, j.MTime))
		}
		// Publish the phase only while this worker still owns the current generation.
		if _, err := p.DB.ExecContext(ctx, "UPDATE jobs SET stage='previews' WHERE asset_id=? AND state='running' AND EXISTS(SELECT 1 FROM assets WHERE id=? AND generation=?)", j.ID, j.ID, j.Generation); err != nil {
			slog.Warn("processing stage update failed", "asset", j.ID, "error", err)
		}
		var temp string
		temp, e = os.MkdirTemp(p.Cache, "processing-")
		if e == nil {
			defer os.RemoveAll(temp)
			preview := source
			if j.Kind == "raw" {
				preview = filepath.Join(temp, "embedded.jpg")
				e = (media.EmbeddedRAW{Runner: p.Runner}).Extract(ctx, source, preview)
				if e == nil && d.Orientation > 1 { // Restore source orientation on the extracted copy only.
					_, e = p.Runner.Run(ctx, "exiftool", "-overwrite_original", fmt.Sprintf("-Orientation#=%d", d.Orientation), preview)
				}
			} else if j.Kind == "video" {
				preview = filepath.Join(temp, "poster.jpg")
				e = video.Poster(ctx, p.Runner, source, preview, d.Duration)
			}
			if e == nil {
				e = thumbnails.Generate(ctx, p.Runner, preview, p.Cache, key)
			}
		}
	}
	if ctx.Err() != nil {
		return
	} // Leave running job for startup recovery.
	if e == nil {
		info, checkErr := os.Stat(source)
		if checkErr != nil {
			e = checkErr
		} else if info.Size() != j.Size || info.ModTime().UnixNano() != j.MTime {
			e = fmt.Errorf("source changed during processing")
		}
	}
	previewStatus, metadataStatus, message := "ready", "ready", ""
	if metaErr != nil {
		metadataStatus = "failed"
		message = metaErr.Error()
		slog.Warn("metadata extraction failed", "asset", j.ID, "error", metaErr)
	}
	if e != nil {
		previewStatus = "failed"
		message += e.Error()
		slog.Warn("thumbnail failed", "asset", j.ID, "error", e)
	}
	place := places.Lookup(d.Latitude, d.Longitude)
	tx, dbErr := p.DB.BeginTx(ctx, nil)
	if dbErr != nil {
		slog.Error("processing persistence failed", "error", dbErr)
		return
	}
	defer tx.Rollback()
	result, dbErr := tx.ExecContext(ctx, `UPDATE assets SET taken_at=?,width=?,height=?,orientation=?,camera_make=?,camera_model=?,lens=?,focal_length=?,aperture=?,exposure_time=?,iso=?,latitude=?,longitude=?,duration=?,video_codec=?,audio_codec=?,preview_key=?,preview_status=?,metadata_status=?,error=?,place_id=?,place_name=?,place_checked=1 WHERE id=? AND generation=?`, d.TakenAt, d.Width, d.Height, d.Orientation, d.CameraMake, d.CameraModel, d.Lens, d.FocalLength, d.Aperture, d.ExposureTime, d.ISO, d.Latitude, d.Longitude, d.Duration, d.VideoCodec, d.AudioCodec, key, previewStatus, metadataStatus, message, place.ID, place.Name, j.ID, j.Generation)
	if dbErr == nil {
		var n int64
		n, dbErr = result.RowsAffected()
		if n == 0 {
			return
		}
	}
	if dbErr == nil {
		if e == nil && metaErr == nil {
			_, dbErr = tx.ExecContext(ctx, "DELETE FROM jobs WHERE asset_id=?", j.ID)
		} else {
			state := "pending"
			if j.Attempts >= 2 {
				state = "failed"
			}
			_, dbErr = tx.ExecContext(ctx, "UPDATE jobs SET state=?,attempts=attempts+1,next_at=? WHERE asset_id=?", state, time.Now().Add(time.Duration(j.Attempts+1)*time.Minute).Unix(), j.ID)
		}
	}
	if dbErr == nil {
		dbErr = tx.Commit()
	}
	if dbErr != nil {
		slog.Error("processing persistence failed", "asset", j.ID, "error", dbErr)
	} else if e == nil {
		slog.Info("thumbnail generated", "asset", j.ID)
	}
}
