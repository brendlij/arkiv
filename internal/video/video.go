package video

import (
	"context"
	"encoding/json"
	"fmt"
	"gallery/internal/metadata"
	"strconv"
	"time"
)

func Probe(ctx context.Context, r metadata.Runner, path string, mtime time.Time) (metadata.Data, error) {
	d := metadata.Data{TakenAt: mtime.UTC().Format(time.RFC3339), Orientation: 1}
	b, e := r.Run(ctx, "ffprobe", "-v", "error", "-show_streams", "-show_format", "-of", "json", path)
	if e != nil {
		return d, e
	}
	var p struct {
		Streams []struct {
			CodecType string            `json:"codec_type"`
			CodecName string            `json:"codec_name"`
			Width     int               `json:"width"`
			Height    int               `json:"height"`
			Tags      map[string]string `json:"tags"`
			SideData  []struct {
				Rotation float64 `json:"rotation"`
			} `json:"side_data_list"`
		} `json:"streams"`
		Format struct {
			Duration string            `json:"duration"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
	}
	if e = json.Unmarshal(b, &p); e != nil {
		return d, e
	}
	d.Duration, _ = strconv.ParseFloat(p.Format.Duration, 64)
	for _, s := range p.Streams {
		if s.CodecType == "video" && d.VideoCodec == "" {
			d.Width = s.Width
			d.Height = s.Height
			d.VideoCodec = s.CodecName
			rotation, _ := strconv.ParseFloat(s.Tags["rotate"], 64)
			for _, side := range s.SideData {
				rotation = side.Rotation
			}
			if int(rotation)%180 != 0 {
				d.Width, d.Height = d.Height, d.Width
			}
			if t, e := metadata.Date(s.Tags["creation_time"]); e == nil {
				d.TakenAt = t.UTC().Format(time.RFC3339)
			}
		}
		if s.CodecType == "audio" {
			d.AudioCodec = s.CodecName
		}
	}
	if t, e := metadata.Date(p.Format.Tags["creation_time"]); e == nil {
		d.TakenAt = t.UTC().Format(time.RFC3339)
	}
	if d.VideoCodec == "" {
		return d, fmt.Errorf("no video stream")
	}
	return d, nil
}
func Poster(ctx context.Context, r metadata.Runner, source, dest string, duration float64) error {
	position := duration * .1
	if position > 30 {
		position = 30
	}
	_, e := r.Run(ctx, "ffmpeg", "-nostdin", "-v", "error", "-threads", "1", "-ss", fmt.Sprintf("%.3f", position), "-i", source, "-frames:v", "1", "-vf", "scale=w='min(2048,iw)':h='min(2048,ih)':force_original_aspect_ratio=decrease", "-threads", "1", "-y", dest)
	return e
}
