package gtfs

// The things that go wrong with timetables.
//
// Three of these are lessons from real feeds rather than invented cases: BART's
// afternoon trains vanished because its times were read as UTC, a mirror served
// a timetable that had expired in June, and night buses are timetabled at 25:10
// rather than 01:10 the next day.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// feedFiles is a minimal but valid feed, parameterised so a test can bend one
// thing at a time.
type feedFiles struct {
	timezone   string
	start, end string
	times      []string // departure times at the single stop
}

func buildZip(t *testing.T, f feedFiles) []byte {
	t.Helper()
	if f.timezone == "" {
		f.timezone = "Europe/London"
	}
	if f.start == "" {
		f.start, f.end = "20200101", "20991231"
	}
	if len(f.times) == 0 {
		f.times = []string{"09:00:00"}
	}

	stopTimes := "trip_id,stop_id,stop_sequence,arrival_time,departure_time\n"
	trips := "route_id,service_id,trip_id,trip_headsign\n"
	for i, tm := range f.times {
		id := fmt.Sprintf("T%d", i)
		trips += fmt.Sprintf("R1,S1,%s,Somewhere\n", id)
		stopTimes += fmt.Sprintf("%s,STOP1,1,%s,%s\n", id, tm, tm)
	}

	files := map[string]string{
		"agency.txt":     "agency_id,agency_name,agency_timezone\nA,Test,{{tz}}\n",
		"stops.txt":      "stop_id,stop_name,stop_lat,stop_lon\nSTOP1,High Street,51.4543,-0.9781\n",
		"routes.txt":     "route_id,route_short_name,route_long_name,route_type\nR1,7,The Seven,3\n",
		"calendar.txt":   "service_id,monday,tuesday,wednesday,thursday,friday,saturday,sunday,start_date,end_date\nS1,1,1,1,1,1,1,1,{{start}},{{end}}\n",
		"trips.txt":      trips,
		"stop_times.txt": stopTimes,
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		body = strings.ReplaceAll(body, "{{tz}}", f.timezone)
		body = strings.ReplaceAll(body, "{{start}}", f.start)
		body = strings.ReplaceAll(body, "{{end}}", f.end)
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func buildInto(t *testing.T, f feedFiles) *Index {
	t.Helper()
	b := buildZip(t, f)
	dir := t.TempDir()
	if _, err := Build("test", bytes.NewReader(b), int64(len(b)), dir, "", ""); err != nil {
		t.Fatal(err)
	}
	idx, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func TestTimesAreReadInTheAgencysZoneNotOurs(t *testing.T) {
	// A feed in Los Angeles, asked at a moment when it is mid-afternoon there
	// and late evening in UTC. Reading the times as UTC puts every departure in
	// the past — which is exactly how BART came back empty.
	idx := buildInto(t, feedFiles{timezone: "America/Los_Angeles", times: []string{"14:30:00", "15:00:00"}})

	stop, _ := idx.Find("High Street")
	at := time.Date(2026, 8, 13, 21, 45, 0, 0, time.UTC) // 14:45 in Los Angeles
	next, err := idx.NextAt(stop, at, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 1 {
		t.Fatalf("want the 15:00 departure and not the 14:30 one, got %d", len(next))
	}
	if got := next[0].When.Format("15:04"); got != "15:00" {
		t.Errorf("departure at %s, want 15:00 local", got)
	}
	// And it is shown in the agency's zone, because that is what is on the board.
	if zone, _ := next[0].When.Zone(); !strings.HasPrefix(zone, "P") {
		t.Errorf("departure rendered in %s, want a Pacific zone", zone)
	}
}

func TestAfterMidnightServicesBelongToYesterday(t *testing.T) {
	// 25:10 is the 1:10am bus on the previous day's timetable. Somebody at the
	// stop at 1am is waiting for it.
	idx := buildInto(t, feedFiles{times: []string{"25:10:00"}})

	stop, _ := idx.Find("High Street")
	at := time.Date(2026, 8, 14, 1, 0, 0, 0, idx.Meta.Location())
	next, err := idx.NextAt(stop, at, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) == 0 {
		t.Fatal("lost the night bus")
	}
	if got := next[0].When.Format("15:04"); got != "01:10" {
		t.Errorf("night bus at %s, want 01:10", got)
	}
}

func TestAnExpiredTimetableSaysSoRatherThanGoingQuiet(t *testing.T) {
	idx := buildInto(t, feedFiles{start: "20260101", end: "20260614"})

	expired, end := idx.Meta.Expired(time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC))
	if !expired {
		t.Fatal("a timetable that ran out in June is not reported as expired")
	}
	if got := end.Format("2006-01-02"); got != "2026-06-14" {
		t.Errorf("expired on %s, want 2026-06-14", got)
	}

	// And a live one is not slandered.
	live := buildInto(t, feedFiles{start: "20260101", end: "20991231"})
	if expired, _ := live.Meta.Expired(time.Now()); expired {
		t.Error("a current timetable reported as expired")
	}
}

func TestABadFeedDoesNotTakeTheGoodOneWithIt(t *testing.T) {
	// This is the whole point of the swap: whatever is answering keeps
	// answering until a replacement has been built and opened.
	good := buildZip(t, feedFiles{times: []string{"09:00:00", "17:00:00"}})

	var serve func(w http.ResponseWriter, r *http.Request)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serve(w, r)
	}))
	defer srv.Close()

	serve = func(w http.ResponseWriter, r *http.Request) { w.Write(good) }

	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Refresh("t", srv.URL); err != nil {
		t.Fatal(err)
	}
	before, ok := s.Get("t")
	if !ok {
		t.Fatal("feed did not load")
	}
	stops := len(before.Meta.Stops)

	// Now the agency starts serving rubbish, in each of the ways it can.
	for _, bad := range []struct {
		name string
		fn   func(w http.ResponseWriter, r *http.Request)
	}{
		{"truncated zip", func(w http.ResponseWriter, r *http.Request) { w.Write(good[:len(good)/3]) }},
		{"not a zip", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>down for maintenance")) }},
		{"empty", func(w http.ResponseWriter, r *http.Request) {}},
		{"server error", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }},
		{"expired timetable", func(w http.ResponseWriter, r *http.Request) {
			w.Write(buildZip(t, feedFiles{start: "20200101", end: "20200201"}))
		}},
	} {
		t.Run(bad.name, func(t *testing.T) {
			serve = bad.fn
			if _, err := s.Refresh("t", srv.URL); err == nil {
				t.Fatal("accepted a broken feed")
			}
			idx, ok := s.Get("t")
			if !ok {
				t.Fatal("lost the feed entirely")
			}
			if len(idx.Meta.Stops) != stops {
				t.Errorf("feed changed under a failed refresh: %d stops, want %d",
					len(idx.Meta.Stops), stops)
			}
			// It must still answer, not merely exist.
			stop, found := idx.Find("High Street")
			if !found {
				t.Fatal("the surviving feed cannot find its own stop")
			}
			if _, err := idx.NextAt(stop, time.Now(), 3); err != nil {
				t.Errorf("the surviving feed stopped answering: %v", err)
			}
			// And no debris is left behind.
			ents, _ := os.ReadDir(dir)
			for _, e := range ents {
				if strings.Contains(e.Name(), ".tmp") || strings.HasSuffix(e.Name(), ".old") {
					t.Errorf("left %s behind", e.Name())
				}
			}
		})
	}
}

func TestAnUnchangedFeedIsNotDownloadedAgain(t *testing.T) {
	body := buildZip(t, feedFiles{})
	var gets, conditional int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gets++
		if r.Header.Get("If-None-Match") == `"v1"` {
			conditional++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Write(body)
	}))
	defer srv.Close()

	s := NewStore(t.TempDir())
	changed, err := s.Refresh("t", srv.URL)
	if err != nil || !changed {
		t.Fatalf("first refresh: changed=%v err=%v", changed, err)
	}
	changed, err = s.Refresh("t", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("rebuilt a feed that had not changed")
	}
	if conditional != 1 {
		t.Errorf("made %d conditional requests, want 1 — the ETag is not being sent", conditional)
	}
}

func TestASecondSourceCoversForTheFirst(t *testing.T) {
	// The agency is down; the mirror answers. This is the only job a mirror has.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer dead.Close()
	alive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(buildZip(t, feedFiles{}))
	}))
	defer alive.Close()

	s := NewStore(t.TempDir())
	changed, err := s.Refresh("t", dead.URL, alive.URL)
	if err != nil || !changed {
		t.Fatalf("fell over instead of falling back: changed=%v err=%v", changed, err)
	}
	if _, ok := s.Get("t"); !ok {
		t.Error("feed not loaded from the fallback")
	}
}

func TestEverySourceFailingSaysWhatWasTried(t *testing.T) {
	s := NewStore(t.TempDir())
	_, err := s.Refresh("t", "http://127.0.0.1:1/a", "http://127.0.0.1:1/b")
	if err == nil {
		t.Fatal("reported success with no sources working")
	}
	for _, want := range []string{"/a", "/b"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
}

func TestAnIndexSurvivesARestart(t *testing.T) {
	b := buildZip(t, feedFiles{times: []string{"09:00:00"}})
	dir := t.TempDir()
	feedDir := filepath.Join(dir, "t")
	if _, err := Build("t", bytes.NewReader(b), int64(len(b)), feedDir, "", ""); err != nil {
		t.Fatal(err)
	}

	// A fresh Store, as though the process had just started.
	s := NewStore(dir)
	idx, ok := s.Get("t")
	if !ok {
		t.Fatal("did not pick up the feed already on disk")
	}
	if _, found := idx.Find("High Street"); !found {
		t.Error("reopened index lost its stops")
	}
}

func TestStopsWithNoDeparturesAreNotOffered(t *testing.T) {
	// Feeds carry entrances, platforms and parent stations that nothing calls
	// at. Offering them as places to wait wastes a caller's next question.
	idx := buildInto(t, feedFiles{})
	near := idx.Near(51.4543, -0.9781, 10)
	if len(near) != 1 {
		t.Fatalf("offered %d stops, want the 1 with departures", len(near))
	}
}

func TestParseTimeKeepsHoursPastMidnight(t *testing.T) {
	for _, c := range []struct {
		in   string
		want uint32
		ok   bool
	}{
		{"09:00:00", 32400, true},
		{"25:10:00", 90600, true},
		{"00:00:00", 0, true},
		{"", 0, false},
		{"nine o'clock", 0, false},
		{"09:70:00", 0, false},
	} {
		got, ok := parseTime(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseTime(%q) = %d,%v want %d,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

// buildFreqZip makes a feed whose one trip is a pattern rather than a run.
func buildFreqZip(t *testing.T, tmpl string, windows []string) []byte {
	t.Helper()
	files := map[string]string{
		"agency.txt":   "agency_id,agency_name,agency_timezone\nA,Test,Europe/London\n",
		"stops.txt":    "stop_id,stop_name,stop_lat,stop_lon\nSTOP1,First,51.4,-0.9\nSTOP2,Second,51.5,-0.8\n",
		"routes.txt":   "route_id,route_short_name,route_type\nR1,7,3\n",
		"calendar.txt": "service_id,monday,tuesday,wednesday,thursday,friday,saturday,sunday,start_date,end_date\nS1,1,1,1,1,1,1,1,20200101,20991231\n",
		"trips.txt":    "route_id,service_id,trip_id,trip_headsign\nR1,S1,T1,Townwards\n",
		// The template: the vehicle takes 10 minutes to reach the second stop.
		"stop_times.txt": "trip_id,stop_id,stop_sequence,arrival_time,departure_time\n" +
			"T1,STOP1,1," + tmpl + "," + tmpl + "\nT1,STOP2,2,06:10:00,06:10:00\n",
		"frequencies.txt": "trip_id,start_time,end_time,headway_secs\n" + strings.Join(windows, "\n") + "\n",
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func openFreq(t *testing.T, b []byte) *Index {
	t.Helper()
	dir := t.TempDir()
	if _, err := Build("f", bytes.NewReader(b), int64(len(b)), dir, "", ""); err != nil {
		t.Fatal(err)
	}
	idx, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func TestAPatternBecomesEveryDepartureInIt(t *testing.T) {
	// Every 10 minutes from 09:00 to 10:00.
	idx := openFreq(t, buildFreqZip(t, "06:00:00", []string{"T1,09:00:00,10:00:00,600"}))

	stop, _ := idx.Find("First")
	at := time.Date(2026, 8, 13, 9, 1, 0, 0, idx.Meta.Location())
	next, err := idx.NextAt(stop, at, 5)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"09:10", "09:20", "09:30", "09:40", "09:50"}
	if len(next) != len(want) {
		t.Fatalf("got %d departures, want %d", len(next), len(want))
	}
	for i, w := range want {
		if got := next[i].When.Format("15:04"); got != w {
			t.Errorf("departure %d at %s, want %s", i, got, w)
		}
	}
}

func TestAPatternIsNotExpandedOntoDisk(t *testing.T) {
	// A five-second headway over twelve hours is 8,640 departures at each of
	// two stops. Storing them would be the bug this design exists to avoid.
	b := buildFreqZip(t, "06:00:00", []string{"T1,06:00:00,18:00:00,5"})
	dir := t.TempDir()
	if _, err := Build("f", bytes.NewReader(b), int64(len(b)), dir, "", ""); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, departuresFile))
	if err != nil {
		t.Fatal(err)
	}
	if rows := fi.Size() / departureSize; rows != 2 {
		t.Errorf("wrote %d rows for a 2-row template, want 2 — the pattern was expanded onto disk", rows)
	}
}

func TestLaterStopsKeepTheirPlaceInThePattern(t *testing.T) {
	// The second stop is ten minutes down the route, so its departures run ten
	// minutes behind the first's.
	idx := openFreq(t, buildFreqZip(t, "06:00:00", []string{"T1,09:00:00,10:00:00,600"}))

	stop, _ := idx.Find("Second")
	at := time.Date(2026, 8, 13, 9, 1, 0, 0, idx.Meta.Location())
	next, err := idx.NextAt(stop, at, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) == 0 {
		t.Fatal("the second stop has no departures")
	}
	if got := next[0].When.Format("15:04"); got != "09:10" {
		t.Errorf("second stop first departure at %s, want 09:10", got)
	}
}

func TestSeveralWindowsInADayAllCount(t *testing.T) {
	// Peak, a gap, then peak again — the shape almost every real feed uses.
	idx := openFreq(t, buildFreqZip(t, "06:00:00", []string{
		"T1,07:00:00,09:00:00,600",
		"T1,16:00:00,18:00:00,300",
	}))

	stop, _ := idx.Find("First")
	// Mid-afternoon: the morning window is done, the evening one has not begun.
	at := time.Date(2026, 8, 13, 14, 0, 0, 0, idx.Meta.Location())
	next, err := idx.NextAt(stop, at, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) == 0 {
		t.Fatal("lost the evening window")
	}
	if got := next[0].When.Format("15:04"); got != "16:00" {
		t.Errorf("first afternoon departure at %s, want 16:00", got)
	}
}

func TestNothingIsDueAfterThePatternEnds(t *testing.T) {
	idx := openFreq(t, buildFreqZip(t, "06:00:00", []string{"T1,09:00:00,10:00:00,600"}))

	stop, _ := idx.Find("First")
	at := time.Date(2026, 8, 13, 11, 0, 0, 0, idx.Meta.Location())
	next, err := idx.NextAt(stop, at, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 0 {
		t.Errorf("invented %d departures after the pattern ended: %v", len(next), next[0].When)
	}
}

func TestTheSameBusIsNotShownTwice(t *testing.T) {
	// Two trips on one route, identical in every way a passenger can see. A
	// feed carries these for directions and day patterns; a departure board
	// must not show four of one bus.
	files := map[string]string{
		"agency.txt":   "agency_id,agency_name,agency_timezone\nA,Test,Europe/London\n",
		"stops.txt":    "stop_id,stop_name,stop_lat,stop_lon\nSTOP1,First,51.4,-0.9\n",
		"routes.txt":   "route_id,route_short_name,route_type\nR1,7,3\n",
		"calendar.txt": "service_id,monday,tuesday,wednesday,thursday,friday,saturday,sunday,start_date,end_date\nS1,1,1,1,1,1,1,1,20200101,20991231\n",
		"trips.txt": "route_id,service_id,trip_id,trip_headsign\n" +
			"R1,S1,T1,Townwards\nR1,S1,T2,Townwards\n",
		"stop_times.txt": "trip_id,stop_id,stop_sequence,arrival_time,departure_time\n" +
			"T1,STOP1,1,09:30:00,09:30:00\nT2,STOP1,1,09:30:00,09:30:00\n",
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	b := buf.Bytes()

	idx := openFreq(t, b)
	stop, _ := idx.Find("First")
	at := time.Date(2026, 8, 13, 9, 0, 0, 0, idx.Meta.Location())
	next, err := idx.NextAt(stop, at, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 1 {
		t.Errorf("showed one bus %d times", len(next))
	}
}

func TestAnExactNameBeatsOneThatMerelyContainsIt(t *testing.T) {
	// The order here is the trap: the wrong feed comes first in the file, which
	// is exactly how "metrobus" found Newfoundland instead of Gatwick.
	catalogMu.Lock()
	old, oldAt := catalog, catalogAt
	catalog = []Feed{
		{ID: "a", Provider: "Metrobus Transit", Country: "CA", Place: "St. John's", Direct: "x"},
		{ID: "b", Provider: "Metrobus", Country: "GB", Place: "Crawley", Direct: "x"},
		{ID: "c", Provider: "Reading Buses", Country: "GB", Place: "Reading", Direct: "x"},
	}
	catalogAt = time.Now()
	catalogMu.Unlock()
	t.Cleanup(func() {
		catalogMu.Lock()
		catalog, catalogAt = old, oldAt
		catalogMu.Unlock()
	})

	f, ok := FindFeed("metrobus")
	if !ok {
		t.Fatal("found nothing")
	}
	if f.Country != "GB" {
		t.Errorf("matched %s in %s, want the exactly-named one in GB", f.Provider, f.Country)
	}

	// An id still wins outright.
	if f, ok := FindFeed("a"); !ok || f.Country != "CA" {
		t.Errorf("an exact id no longer wins: %+v", f)
	}

	// A prefix beats a substring, and a place still matches when nothing else does.
	if f, ok := FindFeed("reading"); !ok || f.ID != "c" {
		t.Errorf("prefix match failed: %+v", f)
	}
	if f, ok := FindFeed("crawley"); !ok || f.ID != "b" {
		t.Errorf("place match failed: %+v", f)
	}
	if _, ok := FindFeed("nowhere at all"); ok {
		t.Error("matched something that is not there")
	}
}
