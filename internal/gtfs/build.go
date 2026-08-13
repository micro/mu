package gtfs

// Turning a published zip into something answerable.
//
// The whole job is to get from eight CSV files to "the next three buses from
// this stop" without ever holding the big one in memory or writing it to disk.
// stop_times.txt is streamed once, straight into a fixed-width array, and the
// array is then sorted by stop so that a query is a seek rather than a scan.

import (
	"archive/zip"
	"encoding/csv"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Meta is everything except the departures: small enough to hold, and
// persisted so a restart does not mean a rebuild.
type Meta struct {
	FeedID    string
	Stops     []Stop
	Routes    []Route
	Trips     []Trip
	Services  []Service
	StopIndex map[string]uint32 // stop id to position in Stops
	// Offsets[i] is where stop i's departures begin in the departures file, and
	// Offsets[i+1] where they end. One extra entry on the end so the last stop
	// needs no special case.
	Offsets []uint32
	// Timezone is the agency's, and it is not optional.
	//
	// Every time in a GTFS feed is local to the agency publishing it, with no
	// offset written down anywhere near it. Comparing those times against the
	// server's clock is the bug that makes a feed look empty: BART's afternoon
	// departures are all in the past if you read them as UTC, so an instance in
	// London reports that no trains run in San Francisco.
	Timezone string
	// Covers is the span the timetable actually describes, as yyyymmdd.
	//
	// Feeds expire. Agencies publish a few weeks at a time and republish before
	// the end, and a mirror that has stopped updating will happily serve a
	// timetable that ran out in June. Without this, an expired feed is
	// indistinguishable from a stop where no buses happen to be due, and the
	// service reports "nothing due" all day instead of "this timetable ended".
	CoversFrom, CoversTo int
	// ETag and Modified are what the feed was serving when it was built, so a
	// refresh can ask whether anything changed before downloading 75MB.
	ETag     string
	Modified string
	BuiltAt  int64
}

// Expired reports whether the timetable has run out, and when it ran out.
func (m *Meta) Expired(now time.Time) (bool, time.Time) {
	if m.CoversTo == 0 {
		return false, time.Time{}
	}
	end := time.Date(m.CoversTo/10000, time.Month(m.CoversTo/100%100), m.CoversTo%100,
		23, 59, 59, 0, m.Location())
	return now.After(end), end
}

// Location is the agency's timezone, falling back to UTC.
func (m *Meta) Location() *time.Location {
	if m.Timezone == "" {
		return time.UTC
	}
	if loc, err := time.LoadLocation(m.Timezone); err == nil {
		return loc
	}
	return time.UTC
}

// reader opens one member of the zip by its base name.
//
// Agencies sometimes nest the files inside a directory, so matching on the base
// name rather than the full path is the difference between reading a feed and
// declaring it malformed.
type archive struct {
	z     *zip.Reader
	files map[string]*zip.File
}

func openArchive(r io.ReaderAt, size int64) (*archive, error) {
	z, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("not a readable zip: %w", err)
	}
	a := &archive{z: z, files: map[string]*zip.File{}}
	for _, f := range z.File {
		base := f.Name
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		if base != "" {
			a.files[strings.ToLower(base)] = f
		}
	}
	return a, nil
}

// each streams one CSV file, calling fn with a column lookup per row.
func (a *archive) each(name string, fn func(get func(string) string) error) error {
	f, ok := a.files[name]
	if !ok {
		return nil // optional files are genuinely optional
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	cr := csv.NewReader(rc)
	cr.ReuseRecord = true
	cr.FieldsPerRecord = -1 // agencies publish ragged rows; the header decides
	cr.LazyQuotes = true

	header, err := cr.Read()
	if err != nil {
		return fmt.Errorf("%s has no header: %w", name, err)
	}
	col := make(map[string]int, len(header))
	for i, h := range header {
		col[strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))] = i
	}

	var rec []string
	get := func(k string) string {
		i, ok := col[k]
		if !ok || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}
	for {
		row, err := cr.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			// One malformed line should not lose a city. Skip it.
			continue
		}
		rec = row
		if err := fn(get); err != nil {
			return err
		}
	}
}

// Build reads a feed zip and writes an index into dir.
//
// dir is expected to be a fresh directory that nothing is reading: the caller
// swaps it into place afterwards, so a failure here leaves the previous index
// untouched.
func Build(feedID string, r io.ReaderAt, size int64, dir string, etag, modified string) (*Meta, error) {
	a, err := openArchive(r, size)
	if err != nil {
		return nil, err
	}
	if _, ok := a.files["stop_times.txt"]; !ok {
		return nil, fmt.Errorf("feed has no stop_times.txt, so it has no timetable in it")
	}

	m := &Meta{
		FeedID: feedID, StopIndex: map[string]uint32{},
		ETag: etag, Modified: modified, BuiltAt: time.Now().Unix(),
	}

	// The agency's zone, which every time in the feed is expressed in.
	if err := a.each("agency.txt", func(get func(string) string) error {
		if m.Timezone == "" {
			m.Timezone = get("agency_timezone")
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Stops.
	if err := a.each("stops.txt", func(get func(string) string) error {
		id := get("stop_id")
		if id == "" {
			return nil
		}
		lat, _ := strconv.ParseFloat(get("stop_lat"), 64)
		lon, _ := strconv.ParseFloat(get("stop_lon"), 64)
		m.StopIndex[id] = uint32(len(m.Stops))
		m.Stops = append(m.Stops, Stop{
			ID: id, Name: get("stop_name"), Code: get("stop_code"), Lat: lat, Lon: lon,
		})
		return nil
	}); err != nil {
		return nil, err
	}
	if len(m.Stops) == 0 {
		return nil, fmt.Errorf("feed has no stops")
	}

	// Routes.
	routeIndex := map[string]uint32{}
	if err := a.each("routes.txt", func(get func(string) string) error {
		id := get("route_id")
		if id == "" {
			return nil
		}
		t, _ := strconv.Atoi(get("route_type"))
		routeIndex[id] = uint32(len(m.Routes))
		m.Routes = append(m.Routes, Route{
			ID: id, Short: get("route_short_name"), Long: get("route_long_name"), Type: t,
		})
		return nil
	}); err != nil {
		return nil, err
	}

	// Calendar, then its exceptions.
	serviceIndex := map[string]uint32{}
	if err := a.each("calendar.txt", func(get func(string) string) error {
		id := get("service_id")
		if id == "" {
			return nil
		}
		var days uint8
		for bit, name := range []string{"monday", "tuesday", "wednesday", "thursday",
			"friday", "saturday", "sunday"} {
			if get(name) == "1" {
				days |= 1 << bit
			}
		}
		start, _ := parseDate(get("start_date"))
		end, _ := parseDate(get("end_date"))
		serviceIndex[id] = uint32(len(m.Services))
		m.Services = append(m.Services, Service{Days: days, Start: start, End: end})
		return nil
	}); err != nil {
		return nil, err
	}

	// Some feeds have no calendar.txt at all and express every day as an
	// exception, so a service may first be seen here.
	if err := a.each("calendar_dates.txt", func(get func(string) string) error {
		id := get("service_id")
		date, err := parseDate(get("date"))
		if id == "" || err != nil {
			return nil
		}
		idx, ok := serviceIndex[id]
		if !ok {
			idx = uint32(len(m.Services))
			serviceIndex[id] = idx
			m.Services = append(m.Services, Service{})
		}
		s := &m.Services[idx]
		switch get("exception_type") {
		case "1":
			if s.Added == nil {
				s.Added = map[int]bool{}
			}
			s.Added[date] = true
		case "2":
			if s.Removed == nil {
				s.Removed = map[int]bool{}
			}
			s.Removed[date] = true
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Trips.
	tripIndex := map[string]uint32{}
	if err := a.each("trips.txt", func(get func(string) string) error {
		id := get("trip_id")
		if id == "" {
			return nil
		}
		route, okR := routeIndex[get("route_id")]
		svc, okS := serviceIndex[get("service_id")]
		if !okR {
			route = 0
		}
		if !okS {
			// A trip whose service is unknown can never be shown to run, which
			// is safer than showing it every day.
			svc = uint32(len(m.Services))
			m.Services = append(m.Services, Service{})
		}
		tripIndex[id] = uint32(len(m.Trips))
		m.Trips = append(m.Trips, Trip{Route: route, Service: svc, Headsign: get("trip_headsign")})
		return nil
	}); err != nil {
		return nil, err
	}

	// stop_times, the big one. Collected as (stop, time, trip) triples, then
	// sorted by stop so each stop's departures end up contiguous on disk.
	type row struct {
		stop uint32
		time uint32
		trip uint32
	}
	rows := make([]row, 0, 1<<20)
	if err := a.each("stop_times.txt", func(get func(string) string) error {
		stop, ok := m.StopIndex[get("stop_id")]
		if !ok {
			return nil
		}
		trip, ok := tripIndex[get("trip_id")]
		if !ok {
			return nil
		}
		// Departure is what somebody waiting cares about. Where a feed gives
		// only an arrival — the last stop of a route, usually — that is what
		// there is.
		t, ok := parseTime(get("departure_time"))
		if !ok {
			if t, ok = parseTime(get("arrival_time")); !ok {
				return nil
			}
		}
		rows = append(rows, row{stop: stop, time: t, trip: trip})
		return nil
	}); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("feed has no usable stop times")
	}

	// What the timetable actually covers, taken from the calendar rather than
	// from what the mirror claims about itself.
	for _, s := range m.Services {
		for _, d := range []int{s.Start, s.End} {
			if d == 0 {
				continue
			}
			if m.CoversFrom == 0 || d < m.CoversFrom {
				m.CoversFrom = d
			}
			if d > m.CoversTo {
				m.CoversTo = d
			}
		}
		for d := range s.Added {
			if m.CoversFrom == 0 || d < m.CoversFrom {
				m.CoversFrom = d
			}
			if d > m.CoversTo {
				m.CoversTo = d
			}
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].stop != rows[j].stop {
			return rows[i].stop < rows[j].stop
		}
		return rows[i].time < rows[j].time
	})

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(dir, departuresFile))
	if err != nil {
		return nil, err
	}
	w := newDepartureWriter(f)
	m.Offsets = make([]uint32, len(m.Stops)+1)
	cur := uint32(0)
	for _, r := range rows {
		for cur <= r.stop {
			m.Offsets[cur] = uint32(w.count)
			cur++
		}
		if err := w.write(Departure{Time: r.time, Trip: r.trip}); err != nil {
			f.Close()
			return nil, err
		}
	}
	for cur <= uint32(len(m.Stops)) {
		m.Offsets[cur] = uint32(w.count)
		cur++
	}
	if err := w.flush(); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	if err := writeMeta(filepath.Join(dir, metaFile), m); err != nil {
		return nil, err
	}
	return m, nil
}

func writeMeta(path string, m *Meta) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(f).Encode(m); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func readMeta(path string) (*Meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var m Meta
	if err := gob.NewDecoder(f).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}
