package places

import (
	"context"
	"gallery/internal/database"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

func TestLookup(t *testing.T) {
	for _, c := range []struct {
		lat, lon float64
		name     string
	}{{52.52, 13.405, "Berlin"}, {48.8566, 2.3522, "Paris"}, {40.7128, -74.006, "New York"}, {-33.8688, 151.2093, "Sydney"}, {0, 0, ""}, {89.9, 179.9, ""}} {
		p := Lookup(&c.lat, &c.lon)
		if (c.name == "" && p.Name != "") || (c.name != "" && !strings.Contains(p.Name, c.name)) {
			t.Errorf("%v: %s", c, p.Name)
		}
	}
	for _, v := range []float64{math.NaN(), math.Inf(1), 91, -91} {
		zero := 0.0
		if Lookup(&v, &zero).ID != "" {
			t.Fatal("invalid GPS accepted")
		}
	}
	if Lookup(nil, nil).ID != "" {
		t.Fatal("missing GPS")
	}
}
func TestBackfillAndSearch(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "places.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, e = db.Exec(`INSERT INTO libraries VALUES('p','Photos','/photos',1);INSERT INTO assets(library_id,relative_path,folder,filename,extension,mime_type,media_type,file_size,modified_at,taken_at,seen_scan,latitude,longitude) VALUES('p','a.jpg','','a.jpg','.jpg','image/jpeg','image',1,1,'2026','s',52.52,13.405),('p','b.jpg','','b.jpg','.jpg','image/jpeg','image',1,1,'2026','s',0,0);`)
	if e != nil {
		t.Fatal(e)
	}
	if e = Backfill(context.Background(), db); e != nil {
		t.Fatal(e)
	}
	var name string
	db.QueryRow("SELECT place_name FROM assets WHERE id=1").Scan(&name)
	if !strings.Contains(name, "Berlin") {
		t.Fatal(name)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM assets_fts WHERE assets_fts MATCH 'Berlin'").Scan(&n)
	if n != 1 {
		t.Fatal("place not searchable")
	}
	db.QueryRow("SELECT count(*) FROM assets WHERE place_checked=1").Scan(&n)
	if n != 2 {
		t.Fatal("unmatched location not marked checked")
	}
	if e = Backfill(context.Background(), db); e != nil {
		t.Fatal(e)
	}
}
func BenchmarkLookup(b *testing.B) {
	lat, lon := 52.52, 13.405
	Lookup(&lat, &lon)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Lookup(&lat, &lon)
	}
}
