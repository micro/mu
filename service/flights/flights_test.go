package flights

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCallsignsTranslatesFlightNumbers(t *testing.T) {
	// The whole reason the airline table is here: a ticket says BA117 and a
	// transponder says BAW117, and nothing derives one from the other.
	cases := []struct {
		in   string
		want string
	}{
		{"BA117", "BAW117"},
		{"ba 117", "BAW117"},
		{"U2 8001", "EZY8001"},
		{"FR1234", "RYR1234"},
		{"AA100", "AAL100"},
		{"LH400", "DLH400"},
	}
	for _, c := range cases {
		got := Callsigns(c.in)
		if len(got) == 0 || got[0] != c.want {
			t.Errorf("Callsigns(%q) = %v, want %q first", c.in, got, c.want)
		}
	}
}

func TestCallsignsKeepsWhatItWasGiven(t *testing.T) {
	// An airline that flies a number under a callsign nobody can derive is
	// still findable by whoever knows the callsign, so the raw string is
	// always tried.
	for _, in := range []string{"BAW117", "SHT4A", "NOTANAIRLINE9"} {
		got := Callsigns(in)
		found := false
		for _, c := range got {
			if c == strings.ToUpper(in) {
				found = true
			}
		}
		if !found {
			t.Errorf("Callsigns(%q) = %v, dropped the query itself", in, got)
		}
	}
}

func TestCallsignsDoesNotTranslateAnICAOPrefix(t *testing.T) {
	// BAW is already a callsign prefix. Reading its first two characters as an
	// IATA code would invent a second, wrong candidate.
	got := Callsigns("BAW117")
	if len(got) != 1 || got[0] != "BAW117" {
		t.Errorf("Callsigns(\"BAW117\") = %v, want just [BAW117]", got)
	}
}

func TestSplitCallsignNamesTheAirline(t *testing.T) {
	a, n := SplitCallsign("BAW117")
	if a == nil || a.IATA != "BA" || n != "117" {
		t.Fatalf("SplitCallsign(\"BAW117\") = %v, %q", a, n)
	}
	if a, _ := SplitCallsign("N628TS"); a != nil {
		t.Errorf("SplitCallsign(\"N628TS\") named an airline for a private tail number: %v", a)
	}
}

func TestLooksLikeRegistration(t *testing.T) {
	for _, reg := range []string{"G-ZBKL", "D-AIMA", "N628TS"} {
		if !looksLikeRegistration(reg) {
			t.Errorf("looksLikeRegistration(%q) = false", reg)
		}
	}
	// A flight number must read as a flight, not a tail number, or every
	// lookup starts by asking the wrong question.
	for _, flight := range []string{"BA117", "BAW117", "FR1234"} {
		if looksLikeRegistration(flight) {
			t.Errorf("looksLikeRegistration(%q) = true, it is a flight", flight)
		}
	}
}

func TestLooksLikeHex(t *testing.T) {
	if !looksLikeHex("406f78") || !looksLikeHex("40756A") {
		t.Error("a six-character hex address was not recognised")
	}
	if looksLikeHex("BAW117") || looksLikeHex("406f7") {
		t.Error("something that is not an ICAO address was read as one")
	}
}

func TestFindAirportPrefersCodesThenMajorAirports(t *testing.T) {
	cases := []struct{ q, wantIATA string }{
		{"LHR", "LHR"},
		{"lhr", "LHR"},
		{"EGLL", "LHR"},
		{"JFK", "JFK"},
		{"Heathrow", "LHR"},
		{"Haneda", "HND"},
	}
	for _, c := range cases {
		got := FindAirport(c.q)
		if got == nil {
			t.Errorf("FindAirport(%q) found nothing", c.q)
			continue
		}
		if got.IATA != c.wantIATA {
			t.Errorf("FindAirport(%q) = %s, want %s", c.q, got.IATA, c.wantIATA)
		}
	}
	if FindAirport("") != nil {
		t.Error("an empty query matched an airport")
	}
}

func TestNearestAirportNamesAPoint(t *testing.T) {
	// A little west of Heathrow.
	ap, dist := NearestAirport(51.47, -0.60)
	if ap == nil {
		t.Fatal("no airport near London")
	}
	if ap.IATA != "LHR" {
		t.Errorf("nearest to west London = %s, want LHR", ap.IATA)
	}
	if dist <= 0 || dist > 30 {
		t.Errorf("distance to Heathrow = %.1f nm, implausible", dist)
	}
}

func TestAircraftTypePassesThroughWhatItCannotName(t *testing.T) {
	if got := AircraftType("B789"); got != "Boeing 787-9" {
		t.Errorf("AircraftType(\"B789\") = %q", got)
	}
	if got := AircraftType("ZZZZ"); got != "ZZZZ" {
		t.Errorf("an unknown designator should come back unchanged, got %q", got)
	}
	if got := AircraftType(""); got != "" {
		t.Errorf("AircraftType(\"\") = %q", got)
	}
}

func TestAltitudeReadsGround(t *testing.T) {
	var a altitude
	if err := json.Unmarshal([]byte(`"ground"`), &a); err != nil || !a.Ground {
		t.Errorf(`"ground" did not read as on the ground: %+v %v`, a, err)
	}
	a = altitude{}
	if err := json.Unmarshal([]byte(`36000`), &a); err != nil || a.Feet != 36000 || a.Ground {
		t.Errorf("36000 did not read as an altitude: %+v %v", a, err)
	}
}

func TestDescribeReadsLikeAnAnswer(t *testing.T) {
	a := Aircraft{
		Callsign: "BAW117", Reg: "G-ZBKL", Type: "B789",
		Altitude: 36000, Speed: 480, Track: 270, Distance: 12, Bearing: 90,
	}
	got := a.Describe(true)
	for _, want := range []string{"British Airways 117", "Boeing 787-9", "36,000 ft", "480 kt", "west", "12 nm east"} {
		if !strings.Contains(got, want) {
			t.Errorf("Describe() = %q, missing %q", got, want)
		}
	}

	ground := Aircraft{Callsign: "EZY21DU", OnGround: true, Speed: 12}
	if !strings.Contains(ground.Describe(false), "on the ground") {
		t.Errorf("a taxiing aircraft did not say so: %q", ground.Describe(false))
	}
}

func TestEmergencySquawksAreNamed(t *testing.T) {
	if got := (Aircraft{Squawk: "7700"}).Emergency(); !strings.Contains(got, "general emergency") {
		t.Errorf("7700 = %q", got)
	}
	if got := (Aircraft{Squawk: "1200"}).Emergency(); got != "" {
		t.Errorf("an ordinary squawk was reported as an emergency: %q", got)
	}
}

func TestClassifySeparatesArrivalsFromOverflights(t *testing.T) {
	// Every airport under a busy airway would otherwise report an arrival for
	// each aeroplane cruising above it.
	all := []Aircraft{
		{Callsign: "A", Altitude: 3000, Climb: -800, Distance: 8},
		{Callsign: "B", Altitude: 4000, Climb: 1800, Distance: 6},
		{Callsign: "C", Altitude: 37000, Climb: -400, Distance: 10},
		{Callsign: "D", OnGround: true, Distance: 1},
		{Callsign: "E", OnGround: true, Distance: 20},
	}
	ground, arriving, departing := classify(all, &Airport{})
	if len(arriving) != 1 || arriving[0].Callsign != "A" {
		t.Errorf("arriving = %v, want just A", names(arriving))
	}
	if len(departing) != 1 || departing[0].Callsign != "B" {
		t.Errorf("departing = %v, want just B", names(departing))
	}
	if len(ground) != 1 || ground[0].Callsign != "D" {
		t.Errorf("ground = %v, want just D — E is 20 nm away and not at this airport", names(ground))
	}
}

func names(list []Aircraft) []string {
	var out []string
	for _, a := range list {
		out = append(out, a.Callsign)
	}
	return out
}

// TestOverheadAgainstAStub exercises the whole path without touching the real
// provider: a test that reaches the network is slow, flaky and rude.
func TestOverheadAgainstAStub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/lat/") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"ac":[
{"hex":"406f78","flight":"BAW117 ","r":"G-ZBKL","t":"B789","alt_baro":36000,"gs":480,"track":270,"baro_rate":0,"lat":51.5,"lon":-0.4,"dst":6.2,"dir":90},
{"hex":"43ea47","flight":"EZY21DU","r":"G-UZHK","t":"A20N","alt_baro":"ground","gs":12,"lat":51.47,"lon":-0.45,"dst":0.4,"dir":180}]}`))
	}))
	defer srv.Close()
	old := adsbBaseURL
	adsbBaseURL, cache, nextSlot = srv.URL, map[string]cacheEntry{}, time.Time{}
	defer func() { adsbBaseURL = old }()

	var rsp OverheadResponse
	if err := (Server{}).Overhead(nil, &OverheadRequest{Near: "LHR", Radius: 20}, &rsp); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Heathrow", "British Airways 117", "Boeing 787-9", "on the ground"} {
		if !strings.Contains(rsp.Text, want) {
			t.Errorf("Overhead text missing %q:\n%s", want, rsp.Text)
		}
	}
	// Nearest first: the one taxiing at 0.4 nm should be above the one at 6.2.
	if i, j := strings.Index(rsp.Text, "EZY"), strings.Index(rsp.Text, "British Airways"); i > j {
		t.Errorf("not sorted nearest first:\n%s", rsp.Text)
	}
}

// TestTrackSaysWhatSilenceMeans is the claim that matters most for a service
// with no schedule behind it. Not finding an aeroplane is not a cancellation,
// and an answer that does not say so is misleading.
func TestTrackSaysWhatSilenceMeans(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"ac":[]}`))
	}))
	defer srv.Close()
	old := adsbBaseURL
	adsbBaseURL, cache, nextSlot = srv.URL, map[string]cacheEntry{}, time.Time{}
	defer func() { adsbBaseURL = old }()

	var rsp TrackResponse
	if err := (Server{}).Track(nil, &TrackRequest{Flight: "BA117"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "BA117") {
		t.Errorf("the answer did not name what was asked for:\n%s", rsp.Text)
	}
	if !strings.Contains(strings.ToLower(rsp.Text), "cancelled") {
		t.Errorf("silence was not distinguished from cancellation:\n%s", rsp.Text)
	}
}

func TestAirportRefusesAnUnknownCode(t *testing.T) {
	var rsp AirportResponse
	if err := (Server{}).Airport(nil, &AirportRequest{Code: "ZZZZZ"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "No airport matches") {
		t.Errorf("an unknown code did not say so:\n%s", rsp.Text)
	}
}

// TestGroundVehiclesAreNotAircraft — Heathrow's control towers transmit, and a
// query centred on the airport came back led by "TWR — TWR, on the ground".
// They carry no emitter category at all, so the source type is what names them.
func TestGroundVehiclesAreNotAircraft(t *testing.T) {
	if !onTheGroundForever("", "adsb_icao_nt") {
		t.Error("a non-transponder ground emitter was read as an aircraft")
	}
	if !onTheGroundForever("C2", "adsb_icao") {
		t.Error("a category C surface vehicle was read as an aircraft")
	}
	// An aircraft that broadcasts no category must survive: plenty do not.
	if onTheGroundForever("", "adsb_icao") || onTheGroundForever("A3", "adsb_icao") {
		t.Error("a real aircraft was filtered out")
	}
}

// TestPacingKeepsUsUnderTheLimit is the bug this exists for. Three lookups fired
// back to back earned a 429, and the caller was told positions were unavailable
// when they were merely being rationed.
func TestPacingKeepsUsUnderTheLimit(t *testing.T) {
	old := minInterval
	minInterval = 40 * time.Millisecond
	defer func() { minInterval = old }()

	var mu sync.Mutex
	var at []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		at = append(at, time.Now())
		mu.Unlock()
		w.Write([]byte(`{"ac":[]}`))
	}))
	defer srv.Close()
	oldURL := adsbBaseURL
	adsbBaseURL, cache, nextSlot = srv.URL, map[string]cacheEntry{}, time.Time{}
	defer func() { adsbBaseURL = oldURL }()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ByCallsign(fmt.Sprintf("TEST%d", i))
		}(i)
	}
	wg.Wait()

	if len(at) != 4 {
		t.Fatalf("got %d requests, want 4", len(at))
	}
	sort.Slice(at, func(i, j int) bool { return at[i].Before(at[j]) })
	for i := 1; i < len(at); i++ {
		if gap := at[i].Sub(at[i-1]); gap < 30*time.Millisecond {
			t.Errorf("requests %d and %d were %v apart, faster than the pace", i-1, i, gap)
		}
	}
}

// TestBusyIsNotUnavailable keeps the two kinds of nothing apart, because one of
// them is worth retrying and the other is not.
func TestBusyIsNotUnavailable(t *testing.T) {
	if problem(errBusy) == problem(errors.New("dial tcp: refused")) {
		t.Error("being throttled and being offline gave the same answer")
	}
	if !strings.Contains(problem(errBusy), "Busy") {
		t.Errorf("problem(errBusy) = %q", problem(errBusy))
	}
}

// TestTheCacheSparesTheProvider — the source is donated hardware, and a page,
// a card and an agent asking within a second must be one request.
func TestTheCacheSparesTheProvider(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Write([]byte(`{"ac":[]}`))
	}))
	defer srv.Close()
	old := adsbBaseURL
	adsbBaseURL, cache, nextSlot = srv.URL, map[string]cacheEntry{}, time.Time{}
	defer func() { adsbBaseURL = old }()

	for i := 0; i < 3; i++ {
		if _, err := Near(51.5, -0.1, 30); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Errorf("three identical questions made %d requests, want 1", calls)
	}
}
