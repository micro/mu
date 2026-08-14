package gtfs

// Asking the index a question.
//
// The departures file is the only thing not held in memory: a stop's entries
// are contiguous and already sorted by time, so answering "what is next here"
// is one seek and a few kilobytes, whatever size the city is.

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	metaFile       = "meta.gob"
	departuresFile = "departures.bin"
	departureSize  = 8
	// maxScan bounds how many of a stop's departures are read in one answer.
	// A busy interchange has thousands a day and a caller wants the next few;
	// this is the difference between reading 8KB and reading a megabyte.
	maxScan = 1024
	// maxStopRows bounds a frequency feed's per-stop scan, which cannot binary
	// search. Singapore, the densest seen, holds about 1,200 rows at its
	// busiest stop.
	maxStopRows = 8192
)

// departureWriter buffers fixed-width records.
type departureWriter struct {
	w     *bufio.Writer
	buf   [departureSize]byte
	count int
}

func newDepartureWriter(f io.Writer) *departureWriter {
	return &departureWriter{w: bufio.NewWriterSize(f, 1<<20)}
}

func (d *departureWriter) write(dep Departure) error {
	binary.LittleEndian.PutUint32(d.buf[0:4], dep.Time)
	binary.LittleEndian.PutUint32(d.buf[4:8], dep.Trip)
	if _, err := d.w.Write(d.buf[:]); err != nil {
		return err
	}
	d.count++
	return nil
}

func (d *departureWriter) flush() error { return d.w.Flush() }

// Index is a built feed, ready to answer.
type Index struct {
	Meta *Meta
	f    *os.File
}

// Open reads an index from a directory.
func Open(dir string) (*Index, error) {
	m, err := readMeta(filepath.Join(dir, metaFile))
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(dir, departuresFile))
	if err != nil {
		return nil, err
	}
	return &Index{Meta: m, f: f}, nil
}

// Close releases the departures file.
func (i *Index) Close() error {
	if i == nil || i.f == nil {
		return nil
	}
	return i.f.Close()
}

// Next is one upcoming departure, resolved into words.
type Next struct {
	Route    string
	Headsign string
	When     time.Time
	In       time.Duration
	Type     int
}

// NextAt returns the coming departures from a stop.
//
// The moment is taken wherever the caller is, and immediately moved into the
// agency's zone, because that is the only zone the feed's numbers mean anything
// in. The times handed back are the agency's too — a Berlin departure board
// read from London still shows Berlin times, because that is what is written
// on it.
func (i *Index) NextAt(stopIdx uint32, at time.Time, limit int) ([]Next, error) {
	if int(stopIdx)+1 >= len(i.Meta.Offsets) {
		return nil, fmt.Errorf("no such stop")
	}
	if limit <= 0 {
		limit = 10
	}
	now := at.In(i.Meta.Location())

	start, end := i.Meta.Offsets[stopIdx], i.Meta.Offsets[stopIdx+1]
	if start >= end {
		return nil, nil
	}

	secs := uint32(now.Hour()*3600 + now.Minute()*60 + now.Second())
	today, weekday := dateInt(now), now.Weekday()

	// Yesterday's late services first: a 25:10 departure belongs to yesterday's
	// timetable and is the 1:10am bus somebody is standing waiting for now.
	var out []Next
	yest := now.AddDate(0, 0, -1)
	out = append(out, i.scan(start, end, secs+86400, dateInt(yest), yest.Weekday(), now, -86400, limit)...)
	out = append(out, i.scan(start, end, secs, today, weekday, now, 0, limit)...)

	sort.Slice(out, func(a, b int) bool { return out[a].When.Before(out[b].When) })

	// The same bus can appear more than once. A feed carries a trip per
	// direction, per day pattern and sometimes per season, and several of them
	// can resolve to the same route leaving the same stop at the same minute —
	// Madrid showed one departure four times over. Nobody standing at the stop
	// believes there are four of them, so the duplicates go.
	seen := make(map[string]bool, len(out))
	kept := out[:0]
	for _, n := range out {
		key := n.Route + "\x00" + n.Headsign + "\x00" + n.When.Format("2006-01-02T15:04")
		if seen[key] {
			continue
		}
		seen[key] = true
		kept = append(kept, n)
	}
	out = kept

	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// scan walks a stop's departures from the first at or after `from`.
func (i *Index) scan(start, end, from uint32, date int, weekday time.Weekday,
	now time.Time, shift int, limit int) []Next {

	n := int(end - start)

	// Where every row holds a real time the slice is sorted, so the first
	// candidate is a binary search. A frequency feed's rows hold offsets
	// instead, which are not in time order, so the whole stop has to be read —
	// affordable because a stop holds hundreds of rows, not millions.
	lo, scan := 0, n
	if len(i.Meta.Windows) == 0 {
		lo = sort.Search(n, func(k int) bool {
			d, err := i.at(start + uint32(k))
			if err != nil {
				return true
			}
			return d.Time >= from
		})
		scan = maxScan
	} else if scan > maxStopRows {
		scan = maxStopRows
	}

	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var out []Next
	for k := lo; k < n && k-lo < scan; k++ {
		if len(out) >= limit && len(i.Meta.Windows) == 0 {
			break
		}
		d, err := i.at(start + uint32(k))
		if err != nil {
			break
		}
		if int(d.Trip) >= len(i.Meta.Trips) {
			continue
		}
		trip := i.Meta.Trips[d.Trip]
		if int(trip.Service) >= len(i.Meta.Services) {
			continue
		}
		if !i.Meta.Services[trip.Service].runsOn(date, weekday) {
			continue
		}

		// One row becomes many when the trip is a pattern rather than a
		// timetabled run.
		times := []uint32{d.Time}
		if windows, ok := i.Meta.Windows[d.Trip]; ok {
			times = times[:0]
			for _, w := range windows {
				times = append(times, w.nextFrom(from, d.Time, limit)...)
			}
		}

		var name string
		var kind int
		if int(trip.Route) < len(i.Meta.Routes) {
			r := i.Meta.Routes[trip.Route]
			name = r.Short
			if name == "" {
				name = r.Long
			}
			kind = r.Type
		}

		for _, t := range times {
			// Rebuild the wall-clock moment. shift moves yesterday's late
			// services back onto the day they were timetabled for.
			when := midnight.Add(time.Duration(int(t)+shift) * time.Second)
			if when.Before(now.Add(-time.Minute)) {
				continue
			}
			out = append(out, Next{
				Route: name, Headsign: trip.Headsign, When: when,
				In: when.Sub(now).Round(time.Minute), Type: kind,
			})
		}
	}
	return out
}

// at reads one departure by absolute position.
func (i *Index) at(pos uint32) (Departure, error) {
	var b [departureSize]byte
	if _, err := i.f.ReadAt(b[:], int64(pos)*departureSize); err != nil {
		return Departure{}, err
	}
	return Departure{
		Time: binary.LittleEndian.Uint32(b[0:4]),
		Trip: binary.LittleEndian.Uint32(b[4:8]),
	}, nil
}

// Near returns stops close to a point, nearest first.
func (i *Index) Near(lat, lon float64, limit int) []Stop {
	if limit <= 0 {
		limit = 10
	}
	type scored struct {
		s Stop
		d float64
	}
	// A stop with no departures is furniture in the feed, not somewhere to
	// wait, so it is not offered.
	var all []scored
	for idx, s := range i.Meta.Stops {
		if i.Meta.Offsets[idx] == i.Meta.Offsets[idx+1] {
			continue
		}
		all = append(all, scored{s: s, d: DistanceKm(lat, lon, s.Lat, s.Lon)})
	}
	sort.Slice(all, func(a, b int) bool { return all[a].d < all[b].d })
	if len(all) > limit {
		all = all[:limit]
	}
	out := make([]Stop, 0, len(all))
	for _, s := range all {
		out = append(out, s.s)
	}
	return out
}

// Find looks a stop up by name, code or id.
func (i *Index) Find(q string) (uint32, bool) {
	q = strings.TrimSpace(q)
	if q == "" {
		return 0, false
	}
	if idx, ok := i.Meta.StopIndex[q]; ok {
		return idx, true
	}
	lower := strings.ToLower(q)

	// Exact name, then code, then prefix, then contains — most specific first,
	// so "Oxford Circus" does not lose to "Oxford Circus Station Stop D".
	var prefix, contains = -1, -1
	for idx, s := range i.Meta.Stops {
		name := strings.ToLower(s.Name)
		switch {
		case name == lower, strings.EqualFold(s.Code, q):
			return uint32(idx), true
		case prefix < 0 && strings.HasPrefix(name, lower):
			prefix = idx
		case contains < 0 && strings.Contains(name, lower):
			contains = idx
		}
	}
	if prefix >= 0 {
		return uint32(prefix), true
	}
	if contains >= 0 {
		return uint32(contains), true
	}
	return 0, false
}

// DistanceKm is the great-circle distance between two points.
func DistanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat, dLon := rad(lat2-lat1), rad(lon2-lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// KindWord names a GTFS route type the way somebody would say it.
func KindWord(t int) string {
	switch t {
	case 0:
		return "tram"
	case 1:
		return "metro"
	case 2:
		return "train"
	case 3:
		return "bus"
	case 4:
		return "ferry"
	case 5:
		return "cable tram"
	case 6:
		return "cable car"
	case 7:
		return "funicular"
	case 11:
		return "trolleybus"
	case 12:
		return "monorail"
	}
	return ""
}
