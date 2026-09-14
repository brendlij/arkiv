package metadata

import (
	"testing"
	"time"
)

func TestParseEXIF(t *testing.T) {
	fallback := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	d, e := Parse([]byte(`[{"DateTimeOriginal":"2025:03:04 12:30:00","OffsetTimeOriginal":"+02:00","Make":"SONY","Model":"ILCE-7CM2","LensModel":"Tamron 28-200mm","ImageWidth":6000,"ImageHeight":4000,"Orientation":6,"GPSLatitude":52.5,"GPSLongitude":13.4,"ISO":320}]`), fallback)
	if e != nil {
		t.Fatal(e)
	}
	if d.TakenAt != "2025-03-04T10:30:00Z" || d.CameraModel != "ILCE-7CM2" || d.Width != 6000 || d.Orientation != 6 || d.ISO != "320" || d.Latitude == nil {
		t.Fatalf("%+v", d)
	}
	d, e = Parse([]byte(`[{}]`), fallback)
	if e != nil || d.TakenAt != fallback.Format(time.RFC3339) {
		t.Fatal("missing EXIF fallback")
	}
	for _, b := range []string{"not json", "[]", `[{"Error":"corrupt"}]`} {
		if _, e = Parse([]byte(b), fallback); e == nil {
			t.Fatalf("accepted malformed %s", b)
		}
	}
	d, e = Parse([]byte(`[{"DateTimeOriginal":"garbage","GPSLatitude":200}]`), fallback)
	if e != nil || d.Latitude != nil || d.TakenAt != fallback.Format(time.RFC3339) {
		t.Fatal("invalid fields not tolerated")
	}
}
