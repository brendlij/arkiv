package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Data struct {
	Width, Height, Orientation                                                       int
	TakenAt, CameraMake, CameraModel, Lens, FocalLength, Aperture, ExposureTime, ISO string
	Latitude, Longitude                                                              *float64
	Duration                                                                         float64
	VideoCodec, AudioCodec                                                           string
}
type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}
type Command struct{}
type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, fmt.Errorf("tool output exceeds limit")
	}
	return b.Buffer.Write(p)
}
func (Command) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var out limitedBuffer
	out.limit = 64 << 20
	var stderr limitedBuffer
	stderr.limit = 8192
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if e := cmd.Run(); e != nil {
		return nil, fmt.Errorf("%s: %w: %s", name, e, stderr.String())
	}
	return out.Bytes(), nil
}
func Extract(ctx context.Context, r Runner, path string, mtime time.Time) (Data, error) {
	b, e := r.Run(ctx, "exiftool", "-json", "-n", "-DateTimeOriginal", "-OffsetTimeOriginal", "-CreateDate", "-Make", "-Model", "-LensModel", "-FocalLength", "-FNumber", "-ExposureTime", "-ISO", "-Orientation", "-ImageWidth", "-ImageHeight", "-GPSLatitude", "-GPSLongitude", path)
	if e != nil {
		return Data{TakenAt: mtime.UTC().Format(time.RFC3339)}, e
	}
	return Parse(b, mtime)
}
func Parse(b []byte, mtime time.Time) (Data, error) {
	d := Data{Orientation: 1, TakenAt: mtime.UTC().Format(time.RFC3339)}
	var rows []map[string]any
	if e := json.Unmarshal(b, &rows); e != nil {
		return d, e
	}
	if len(rows) != 1 {
		return d, fmt.Errorf("metadata response must contain one record")
	}
	m := rows[0]
	if v := text(m, "Error"); v != "" {
		return d, fmt.Errorf("metadata: %s", v)
	}
	d.Width = int(number(m, "ImageWidth"))
	d.Height = int(number(m, "ImageHeight"))
	if o := int(number(m, "Orientation")); o >= 1 && o <= 8 {
		d.Orientation = o
	}
	d.CameraMake = text(m, "Make")
	d.CameraModel = text(m, "Model")
	d.Lens = text(m, "LensModel")
	d.FocalLength = text(m, "FocalLength")
	d.Aperture = text(m, "FNumber")
	d.ExposureTime = text(m, "ExposureTime")
	d.ISO = text(m, "ISO")
	if x, ok := m["GPSLatitude"].(float64); ok && x >= -90 && x <= 90 {
		d.Latitude = &x
	}
	if x, ok := m["GPSLongitude"].(float64); ok && x >= -180 && x <= 180 {
		d.Longitude = &x
	}
	for _, key := range []string{"DateTimeOriginal", "CreateDate"} {
		v := text(m, key)
		if key == "DateTimeOriginal" {
			v += text(m, "OffsetTimeOriginal")
		}
		if t, e := Date(v); e == nil {
			d.TakenAt = t.UTC().Format(time.RFC3339)
			break
		}
	}
	return d, nil
}
func Date(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006:01:02 15:04:05Z07:00", "2006:01:02 15:04:05.999999999Z07:00", "2006:01:02 15:04:05", "2006-01-02 15:04:05"} {
		if t, e := time.ParseInLocation(layout, strings.TrimSpace(s), time.Local); e == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid capture date")
}
func text(m map[string]any, k string) string {
	if v, ok := m[k]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}
func number(m map[string]any, k string) float64 { v, _ := m[k].(float64); return v }
