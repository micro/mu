package flights

// The provider: adsb.lol, a volunteer ADS-B aggregator.
//
// It needs no key, no account and no card, which is the only reason a flights
// service is worth having here at all. The commercial alternatives — FlightAware,
// Cirium, AviationStack — each want a signup and a card before they will tell
// you where an aeroplane is, and that is precisely the barrier this instance
// exists to absorb.
//
// Two consequences follow from the source being a volunteer network rather than
// a purchased feed, and both are honest limits rather than bugs to fix.
// Coverage is where receivers are: dense over Europe and North America, thin
// over oceans, absent over much of Africa and central Asia. And there is no
// schedule — an aeroplane that has not taken off is not transmitting a position,
// so it cannot be found, and "not found" never means "cancelled".
//
// Because it costs nothing it also earns nothing, so this cache is not a cost
// control. It is manners: a page render, a card and an agent asking the same
// question a second apart should be one request to somebody else's donated
// hardware, not three.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// adsbBaseURL is a variable so tests can point it at a stub. A test that reaches
// the real network is slow, flaky and rude to a free provider.
var adsbBaseURL = "https://api.adsb.lol/v2"

const (
	// positionTTL is how long a set of positions is reused. An airliner at
	// cruise covers about four nautical miles in fifteen seconds, which is
	// invisible in an answer that reports distance to the nearest mile.
	positionTTL = 15 * time.Second

	// maxCacheEntries bounds the cache. A caller asking about a thousand random
	// coordinates must not be able to grow it without limit.
	maxCacheEntries = 400

	// maxRadiusNM is the provider's own ceiling on a radius query.
	maxRadiusNM = 250

	// userAgent identifies Mu to the provider. An anonymous flood is what gets
	// a free API closed; a named one can at least be asked to stop.
	userAgent = "mu (https://micro.mu)"
)

// altitude reads the upstream's altitude field, which is a number in feet when
// the aircraft is flying and the string "ground" when it is not. Both are
// meaningful, so neither can simply be dropped.
type altitude struct {
	Feet   int
	Ground bool
}

func (a *altitude) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "ground" {
		a.Ground = true
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil // an altitude we cannot read is an absent altitude, not an error
	}
	a.Feet = int(f)
	return nil
}

// adsbAircraft is the subset of the upstream record Mu reads.
type adsbAircraft struct {
	Hex      string   `json:"hex"`
	Source   string   `json:"type"` // how the position was heard, not the aircraft type
	Flight   string   `json:"flight"`
	Reg      string   `json:"r"`
	Type     string   `json:"t"`
	AltBaro  altitude `json:"alt_baro"`
	Speed    float64  `json:"gs"`
	Track    float64  `json:"track"`
	BaroRate float64  `json:"baro_rate"`
	Squawk   string   `json:"squawk"`
	Category string   `json:"category"`
	Lat      float64  `json:"lat"`
	Lon      float64  `json:"lon"`
	SeenPos  float64  `json:"seen_pos"`
	Dst      float64  `json:"dst"`
	Dir      float64  `json:"dir"`
}

type adsbResponse struct {
	AC []adsbAircraft `json:"ac"`
}

// onTheGroundForever reports whether a record belongs to something that is not
// an aircraft and never will be.
//
// ADS-B is not only aeroplanes. Airports fit transmitters to pushback tugs, fire
// appliances and de-icing rigs so that ground radar can see them, and the control
// tower itself transmits as a fixed obstruction. A query centred on Heathrow came
// back led by "TWR — TWR, on the ground", which is the tower correctly reporting
// that it is a building.
//
// Two signals, because the obvious one is not enough. ICAO's emitter categories
// put surface vehicles and fixed obstructions in category C, so C goes — but the
// Heathrow towers broadcast no category at all. What they do carry is the
// non-transponder source type, which is exactly what a ground emitter uses: it
// has no Mode S transponder because it is not an aircraft.
//
// Records with neither signal are kept. Plenty of real aircraft send no
// category, and dropping the unlabelled would lose more than it cleans up.
func onTheGroundForever(category, source string) bool {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(category)), "C") {
		return true
	}
	return strings.TrimSpace(source) == "adsb_icao_nt"
}

func (r adsbAircraft) toAircraft() Aircraft {
	return Aircraft{
		Hex:      strings.ToUpper(strings.TrimSpace(r.Hex)),
		Callsign: strings.ToUpper(strings.TrimSpace(r.Flight)),
		Reg:      strings.ToUpper(strings.TrimSpace(r.Reg)),
		Type:     strings.ToUpper(strings.TrimSpace(r.Type)),
		Lat:      r.Lat,
		Lon:      r.Lon,
		Altitude: r.AltBaro.Feet,
		OnGround: r.AltBaro.Ground,
		Speed:    r.Speed,
		Track:    r.Track,
		Climb:    r.BaroRate,
		Squawk:   r.Squawk,
		Distance: r.Dst,
		Bearing:  r.Dir,
		Age:      r.SeenPos,
	}
}

var (
	cacheMu sync.Mutex
	cache   = map[string]cacheEntry{}
)

type cacheEntry struct {
	aircraft []Aircraft
	at       time.Time
}

// The pace at which this instance is allowed to talk to the provider.
//
// This was not a design decision made up front, it was a bug. Looking up "BA117"
// tries the translated callsign, then the raw one, then the registration —
// three requests in as many milliseconds — and the fourth came back 429. The
// answer the caller saw was "live positions are unavailable", which was untrue:
// they were available, and Mu had been rude to a free service and been told so.
//
// So requests queue. One at a time, roughly a second apart, for the whole
// process rather than per caller, because the provider counts by source and not
// by goroutine. The queue is bounded: waiting four seconds for a position is
// worse than saying it is busy.
var (
	gate     sync.Mutex
	nextSlot time.Time
)

// minInterval is the provider's published rate for the free v2 API, with a
// little margin. Anything faster is borrowing against a 429. A variable so a
// test can drop it: a suite that paces itself politely at a stub server is
// spending seconds to be polite to nobody.
var minInterval = 1100 * time.Millisecond

// maxQueueWait is how long a caller will queue before being told the service is
// busy rather than left holding a page open.
const maxQueueWait = 4 * time.Second

// reserve claims the next slot in the queue and reports how long to wait for it.
// The sleep happens outside the lock so that queued callers each hold their own
// place rather than the mutex.
func reserve() (time.Duration, bool) {
	gate.Lock()
	defer gate.Unlock()
	now := time.Now()
	if nextSlot.Before(now) {
		nextSlot = now
	}
	wait := nextSlot.Sub(now)
	if wait > maxQueueWait {
		return 0, false
	}
	nextSlot = nextSlot.Add(minInterval)
	return wait, true
}

// errBusy means the queue was too long, not that the provider failed.
var errBusy = fmt.Errorf("adsb: too many requests queued")

// fetch calls one upstream path, through the cache.
func fetch(path string) ([]Aircraft, error) {
	cacheMu.Lock()
	if e, ok := cache[path]; ok && time.Since(e.at) < positionTTL {
		cacheMu.Unlock()
		return e.aircraft, nil
	}
	cacheMu.Unlock()

	wait, ok := reserve()
	if !ok {
		return nil, errBusy
	}
	time.Sleep(wait)

	req, err := http.NewRequest(http.MethodGet, adsbBaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	rsp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode == http.StatusTooManyRequests {
		// The pacing above should make this unreachable. It is here because the
		// provider's limit is theirs to change, and because other things share
		// an address in front of it.
		return nil, errBusy
	}
	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("adsb: %s", rsp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(rsp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var parsed adsbResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	out := make([]Aircraft, 0, len(parsed.AC))
	for _, a := range parsed.AC {
		if onTheGroundForever(a.Category, a.Source) {
			continue
		}
		out = append(out, a.toAircraft())
	}

	cacheMu.Lock()
	if len(cache) >= maxCacheEntries {
		cache = map[string]cacheEntry{}
	}
	cache[path] = cacheEntry{aircraft: out, at: time.Now()}
	cacheMu.Unlock()
	return out, nil
}

// Near returns the aircraft within radius nautical miles of a point, nearest
// first. Distance and bearing come back already computed relative to the point
// asked about.
func Near(lat, lon float64, radiusNM int) ([]Aircraft, error) {
	if radiusNM <= 0 {
		radiusNM = 50
	}
	if radiusNM > maxRadiusNM {
		radiusNM = maxRadiusNM
	}
	return fetch(fmt.Sprintf("/lat/%.4f/lon/%.4f/dist/%d", lat, lon, radiusNM))
}

// ByCallsign returns the aircraft currently transmitting a callsign. Callsigns
// are not unique over time — yesterday's BAW117 is a different aeroplane — but
// they are unique in the air right now, which is the only tense this answers in.
func ByCallsign(cs string) ([]Aircraft, error) {
	return fetch("/callsign/" + strings.ToUpper(cs))
}

// ByRegistration returns the aircraft wearing a registration, e.g. "G-ZBKL".
func ByRegistration(reg string) ([]Aircraft, error) {
	return fetch("/registration/" + strings.ToUpper(reg))
}

// ByHex returns the aircraft with an ICAO 24-bit address, e.g. "406F78". This is
// the only identifier that is genuinely permanent: it is burned into the
// transponder and follows the airframe.
func ByHex(hex string) ([]Aircraft, error) {
	return fetch("/hex/" + strings.ToLower(hex))
}
