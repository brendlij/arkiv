// Package places names coordinates using a bundled GeoNames dataset. No network calls.
package places

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	_ "embed"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed cities.tsv.gz
var gazetteer []byte

type Place struct {
	ID, Name string
	point    [3]float64
}
type node struct {
	place       Place
	axis        int
	left, right *node
}

var once sync.Once
var tree *node

func unit(lat, lon float64) [3]float64 {
	lat *= math.Pi / 180
	lon *= math.Pi / 180
	return [3]float64{math.Cos(lat) * math.Cos(lon), math.Cos(lat) * math.Sin(lon), math.Sin(lat)}
}
func build(p []Place, depth int) *node {
	if len(p) == 0 {
		return nil
	}
	axis := depth % 3
	sort.Slice(p, func(i, j int) bool { return p[i].point[axis] < p[j].point[axis] })
	mid := len(p) / 2
	return &node{p[mid], axis, build(p[:mid], depth+1), build(p[mid+1:], depth+1)}
}
func load() {
	reader, e := gzip.NewReader(bytes.NewReader(gazetteer))
	if e != nil {
		panic(e)
	}
	defer reader.Close()
	b, e := io.ReadAll(reader)
	if e != nil {
		panic(e)
	}
	items := []Place{}
	for _, line := range strings.Split(string(b), "\n") {
		r := strings.Split(line, "\t")
		if len(r) != 4 {
			continue
		}
		lat, e := strconv.ParseFloat(r[1], 64)
		if e != nil {
			panic(e)
		}
		lon, e := strconv.ParseFloat(r[2], 64)
		if e != nil {
			panic(e)
		}
		items = append(items, Place{r[0], r[3], unit(lat, lon)})
	}
	tree = build(items, 0)
}
func nearest(n *node, q [3]float64, best *Place, distance *float64) {
	if n == nil {
		return
	}
	d := 0.0
	for i := 0; i < 3; i++ {
		delta := q[i] - n.place.point[i]
		d += delta * delta
	}
	if d < *distance {
		*distance = d
		*best = n.place
	}
	delta := q[n.axis] - n.place.point[n.axis]
	first, second := n.left, n.right
	if delta > 0 {
		first, second = second, first
	}
	nearest(first, q, best, distance)
	if delta*delta < *distance {
		nearest(second, q, best, distance)
	}
}
func Lookup(lat, lon *float64) Place {
	if lat == nil || lon == nil || math.IsNaN(*lat) || math.IsNaN(*lon) || math.IsInf(*lat, 0) || math.IsInf(*lon, 0) || *lat < -90 || *lat > 90 || *lon < -180 || *lon > 180 {
		return Place{}
	}
	once.Do(load)
	var best Place
	distance := math.Inf(1)
	nearest(tree, unit(*lat, *lon), &best, &distance)
	km := 2 * 6371 * math.Asin(math.Min(1, math.Sqrt(distance)/2))
	if km > 75 {
		return Place{}
	}
	return best
}

// Backfill updates only derived database metadata, in bounded transactions.
func Backfill(ctx context.Context, db *sql.DB) error {
	for {
		rows, e := db.QueryContext(ctx, "SELECT id,latitude,longitude FROM assets WHERE place_checked=0 AND latitude IS NOT NULL AND longitude IS NOT NULL LIMIT 200")
		if e != nil {
			return e
		}
		type update struct {
			id       int64
			lat, lon float64
			p        Place
		}
		items := []update{}
		for rows.Next() {
			var u update
			if e = rows.Scan(&u.id, &u.lat, &u.lon); e != nil {
				rows.Close()
				return e
			}
			u.p = Lookup(&u.lat, &u.lon)
			items = append(items, u)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(items) == 0 {
			return nil
		}
		tx, e := db.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		for _, u := range items {
			_, e = tx.ExecContext(ctx, "UPDATE assets SET place_id=?,place_name=?,place_checked=1 WHERE id=? AND latitude=? AND longitude=?", u.p.ID, u.p.Name, u.id, u.lat, u.lon)
			if e != nil {
				tx.Rollback()
				return e
			}
		}
		if e = tx.Commit(); e != nil {
			return e
		}
	}
}
