package api

import (
	"fmt"
	"gallery/internal/access"
	"math"
	"net/http"
	"strconv"
	"strings"
)

const validGPS = "a.latitude BETWEEN -90 AND 90 AND a.longitude BETWEEN -180 AND 180"

type bounds struct{ west, south, east, north float64 }

func parseBounds(v string) (bounds, error) {
	var b bounds
	p := strings.Split(v, ",")
	if len(p) != 4 {
		return b, fmt.Errorf("bbox must be west,south,east,north")
	}
	vals := []*float64{&b.west, &b.south, &b.east, &b.north}
	for i, s := range p {
		n, e := strconv.ParseFloat(s, 64)
		if e != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return b, fmt.Errorf("invalid bounds")
		}
		*vals[i] = n
	}
	if b.west < -180 || b.west > 180 || b.east < -180 || b.east > 180 || b.south < -90 || b.north > 90 || b.south > b.north {
		return b, fmt.Errorf("invalid bounds")
	}
	return b, nil
}
func (b bounds) sql() (string, []any) {
	lon := "(a.longitude>=? AND a.longitude<=?)"
	if b.west > b.east {
		lon = "(a.longitude>=? OR a.longitude<=?)"
	}
	return "(" + validGPS + " AND a.latitude>=? AND a.latitude<=? AND " + lon + ")", []any{b.south, b.north, b.west, b.east}
}
func (s *Server) mapPoints(w http.ResponseWriter, r *http.Request) {
	box := bounds{-180, -90, 180, 90}
	var e error
	if v := r.URL.Query().Get("bbox"); v != "" {
		box, e = parseBounds(v)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
	}
	zoom := 2
	if v := r.URL.Query().Get("zoom"); v != "" {
		zoom, e = strconv.Atoi(v)
		if e != nil || zoom < 1 || zoom > 18 {
			http.Error(w, "zoom must be 1..18", 400)
			return
		}
	}
	cell := 360 / math.Pow(2, float64(zoom)) * 80 / 256
	condition, args := box.sql()
	where := "l.enabled=1 AND " + access.Visible(access.Current(r), "a")
	var tagged, total int64
	e = s.DB.QueryRowContext(r.Context(), "SELECT count(*),coalesce(sum("+validGPS+"),0) FROM assets a JOIN libraries l ON l.id=a.library_id WHERE "+where).Scan(&total, &tagged)
	if e != nil {
		s.fail(w, e)
		return
	}
	args = append(args, cell, cell)
	rows, e := s.DB.QueryContext(r.Context(), `SELECT avg(a.latitude),avg(a.longitude),count(*),min(a.id),min(a.latitude),min(a.longitude),max(a.latitude),max(a.longitude),CASE WHEN min(a.place_name)=max(a.place_name) THEN min(a.place_name) ELSE '' END FROM assets a JOIN libraries l ON l.id=a.library_id WHERE `+where+` AND `+condition+` GROUP BY cast((a.latitude+90)/? AS INTEGER),cast((a.longitude+180)/? AS INTEGER) ORDER BY count(*) DESC,min(a.id) LIMIT 1001`, args...)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	type point struct {
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
		Count   int     `json:"count"`
		AssetID int64   `json:"assetId"`
		South   float64 `json:"south"`
		West    float64 `json:"west"`
		North   float64 `json:"north"`
		East    float64 `json:"east"`
		Name    string  `json:"name"`
	}
	items := []point{}
	for rows.Next() {
		var p point
		if e = rows.Scan(&p.Lat, &p.Lon, &p.Count, &p.AssetID, &p.South, &p.West, &p.North, &p.East, &p.Name); e != nil {
			s.fail(w, e)
			return
		}
		items = append(items, p)
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	more := len(items) > 1000
	if more {
		items = items[:1000]
	}
	send(w, map[string]any{"items": items, "hasMore": more, "geotagged": tagged, "withoutGPS": total - tagged})
}
func (s *Server) placeList(w http.ResponseWriter, r *http.Request) {
	offset := 0
	var e error
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, e = strconv.Atoi(v)
		if e != nil || offset < 0 {
			http.Error(w, "invalid offset", 400)
			return
		}
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 200 {
		http.Error(w, "search too long", 400)
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), `SELECT a.place_id,a.place_name,count(*),avg(a.latitude),avg(a.longitude) FROM assets a JOIN libraries l ON l.id=a.library_id WHERE l.enabled=1 AND `+access.Visible(access.Current(r), "a")+` AND `+validGPS+` AND a.place_id<>'' AND instr(lower(a.place_name),lower(?))>0 GROUP BY a.place_id,a.place_name ORDER BY count(*) DESC,a.place_name,a.place_id LIMIT 101 OFFSET ?`, q, offset)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	type place struct {
		ID    string  `json:"id"`
		Name  string  `json:"name"`
		Count int     `json:"count"`
		Lat   float64 `json:"lat"`
		Lon   float64 `json:"lon"`
	}
	items := []place{}
	for rows.Next() {
		var p place
		if e = rows.Scan(&p.ID, &p.Name, &p.Count, &p.Lat, &p.Lon); e != nil {
			s.fail(w, e)
			return
		}
		items = append(items, p)
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	more := len(items) > 100
	if more {
		items = items[:100]
	}
	send(w, map[string]any{"items": items, "hasMore": more})
}
