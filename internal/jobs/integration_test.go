package jobs

import (
	"context"
	"crypto/sha256"
	"gallery/internal/metadata"
	"gallery/internal/thumbnails"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This test uses actual binaries and generated media, never mock processing output.
func TestRealMediaPipeline(t *testing.T) {
	if os.Getenv("ARKIV_INTEGRATION") != "1" {
		t.Skip("set ARKIV_INTEGRATION=1 with ExifTool, libvips and ffmpeg installed")
	}
	for _, tool := range []string{"exiftool", "vipsthumbnail", "vipsheader", "ffmpeg", "ffprobe"} {
		if _, e := exec.LookPath(tool); e != nil {
			t.Fatal(e)
		}
	}
	p, s := setup(t)
	p.Runner = metadata.Command{}
	ctx := context.Background()
	root := s.Libraries[0].Root
	img := image.NewRGBA(image.Rect(0, 0, 160, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 160; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y * 2), 120, 255})
		}
	}
	source := filepath.Join(root, "a.jpg")
	f, e := os.Create(source)
	if e != nil {
		t.Fatal(e)
	}
	e = jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
	f.Close()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = p.Runner.Run(ctx, "exiftool", "-overwrite_original", "-Make=SONY", "-Model=ILCE-7CM2", "-LensModel=Test lens", "-DateTimeOriginal=2025:09:13 07:34:00", "-OffsetTimeOriginal=+02:00", "-Orientation#=6", source); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(source)
	if e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(before)
	for _, ext := range []string{"mp4", "mov"} {
		dest := filepath.Join(root, "clip."+ext)
		if _, e = p.Runner.Run(ctx, "ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=160x100:rate=10", "-t", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-metadata", "creation_time=2025-09-12T10:00:00Z", "-threads", "1", "-y", dest); e != nil {
			t.Fatal(e)
		}
	}
	if e = os.WriteFile(filepath.Join(root, "broken.jpg"), []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Scan(ctx); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 4; i++ {
		j, e := p.claim(ctx)
		if e != nil {
			t.Fatal(e)
		}
		p.process(ctx, j)
	}
	var ready, failed int
	p.DB.QueryRow("SELECT count(*) FROM assets WHERE preview_status='ready'").Scan(&ready)
	p.DB.QueryRow("SELECT count(*) FROM assets WHERE preview_status='failed'").Scan(&failed)
	if ready != 3 || failed != 1 {
		rows, _ := p.DB.Query("SELECT filename,error FROM assets")
		defer rows.Close()
		for rows.Next() {
			var name, message string
			rows.Scan(&name, &message)
			t.Log(name, message)
		}
		t.Fatalf("ready=%d failed=%d", ready, failed)
	}
	var key, camera, taken string
	var videoKey string
	if e = p.DB.QueryRow("SELECT preview_key FROM assets WHERE filename='clip.mp4'").Scan(&videoKey); e != nil {
		t.Fatal(e)
	}
	videoWidth, e := p.Runner.Run(ctx, "vipsheader", "-f", "width", thumbnails.Path(p.Cache, videoKey, 1024))
	if e != nil || strings.TrimSpace(string(videoWidth)) != "160" {
		t.Fatal("video poster upscaled", string(videoWidth), e)
	}
	p.DB.QueryRow("SELECT preview_key,camera_model,taken_at FROM assets WHERE filename='a.jpg'").Scan(&key, &camera, &taken)
	if camera != "ILCE-7CM2" || !strings.HasPrefix(taken, "2025-09-13T05:34:00") {
		t.Fatal(camera, taken)
	}
	for _, size := range thumbnails.Sizes {
		path := thumbnails.Path(p.Cache, key, size)
		b, e := p.Runner.Run(ctx, "vipsheader", "-f", "width", path)
		if e != nil || strings.TrimSpace(string(b)) != "100" {
			t.Fatalf("oriented width/upscale: %s %v", b, e)
		}
		b, e = p.Runner.Run(ctx, "vipsheader", "-f", "height", path)
		if e != nil || strings.TrimSpace(string(b)) != "160" {
			t.Fatalf("oriented height/upscale: %s %v", b, e)
		}
	}
	after, e := os.ReadFile(source)
	if e != nil || sha256.Sum256(after) != hash {
		t.Fatal("original modified")
	}
	r, e := s.Scan(ctx)
	if e != nil || r.Discovered != 0 || r.Updated != 0 {
		t.Fatalf("unnecessary reprocessing %+v %v", r, e)
	}
	p.DB.Exec("DELETE FROM jobs WHERE state='pending'")
	p.DB.Exec("INSERT INTO jobs(asset_id,state) SELECT id,'running' FROM assets WHERE filename='a.jpg'")
	run, cancel := context.WithTimeout(ctx, 15*time.Second)
	if e = p.Start(run); e != nil {
		t.Fatal(e)
	}
	for run.Err() == nil {
		var n int
		p.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&n)
		if n == 0 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	cancel()
	p.Wait()
	var n int
	p.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&n)
	if n != 0 {
		t.Fatal("restart recovery failed")
	}
}
