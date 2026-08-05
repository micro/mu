// Package usage counts what this instance is being asked to do.
//
// The operator could already see who had signed up and what Mu had spent on
// third-party APIs, but not what anyone actually did: no requests per minute,
// no busiest endpoint, no idea which account or which agent was driving the
// load. Logs are not an answer to that — they say what happened once, not how
// much of it is happening.
//
// This is counters, not events. Every request adds one to three buckets (this
// minute, this hour, this day) against three dimensions — the surface it came
// through, the endpoint or tool it asked for, and the account it belonged to.
// Nothing is stored per request, so there is no log to grow, no PII beyond the
// account id that is already on every other admin page, and the cost is a map
// increment under a mutex.
//
// Deliberately no percentiles, no cardinality explosion, no time-series
// database. If this instance ever needs that, it needs a real one, not a worse
// one grown here.
package usage

import (
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
)

// Resolution is one of the three time bases a count is kept at.
type Resolution string

const (
	Minute Resolution = "minute"
	Hour   Resolution = "hour"
	Day    Resolution = "day"
)

// How much history each resolution keeps. Two hours of minutes answers "what is
// happening right now", a week of hours answers "when is it busy", and a
// quarter of days answers "is this growing".
const (
	keepMinutes = 120
	keepHours   = 24 * 7
	keepDays    = 90

	// maxKeys bounds one bucket's breakdown. Endpoints and tools are a fixed
	// set, but an app slug or a bad crawler is not — anything past the cap is
	// counted under "other" so a single visitor cannot grow the file.
	maxKeys = 200

	// Other is where counts land once a bucket is full.
	Other = "other"

	storeKey = "usage/counters.json"
)

// Bucket is one interval's counts.
type Bucket struct {
	At       time.Time      `json:"at"`
	Total    int            `json:"total"`
	Names    map[string]int `json:"names,omitempty"`
	Users    map[string]int `json:"users,omitempty"`
	Surfaces map[string]int `json:"surfaces,omitempty"`
}

type ring struct {
	Step    time.Duration `json:"step"`
	Keep    int           `json:"keep"`
	Buckets []Bucket      `json:"buckets"` // oldest first
}

type store struct {
	Minute *ring `json:"minute"`
	Hour   *ring `json:"hour"`
	Day    *ring `json:"day"`
}

var (
	mu    sync.Mutex
	rings = newStore()
	dirty bool

	// now is overridable so tests can roll time forward.
	now = time.Now
)

func newStore() store {
	return store{
		Minute: &ring{Step: time.Minute, Keep: keepMinutes},
		Hour:   &ring{Step: time.Hour, Keep: keepHours},
		Day:    &ring{Step: 24 * time.Hour, Keep: keepDays},
	}
}

// Load restores the counters and starts the periodic save.
func Load() {
	restore()
	go saver()
}

// restore reads the counters back from disk. Separate from Load so a test can
// simulate a restart without starting the saver.
func restore() {
	var s store
	if err := data.LoadJSON(storeKey, &s); err == nil && s.Minute != nil && s.Hour != nil && s.Day != nil {
		mu.Lock()
		rings = s
		// Step and Keep are behaviour, not data: take them from the code so
		// changing a retention window applies to what is already stored.
		rings.Minute.Step, rings.Minute.Keep = time.Minute, keepMinutes
		rings.Hour.Step, rings.Hour.Keep = time.Hour, keepHours
		rings.Day.Step, rings.Day.Keep = 24*time.Hour, keepDays
		mu.Unlock()
	}
}

// Record counts one call.
//
//   - surface is how it arrived: "web", "mcp", "cli", "app".
//   - name is what was asked for: an endpoint, a tool name, an app slug.
//   - account is who asked, empty for a signed-out visitor.
func Record(surface, name, account string) {
	if name == "" {
		return
	}
	if account == "" {
		account = "guest"
	}
	t := now().UTC()

	mu.Lock()
	defer mu.Unlock()
	for _, r := range []*ring{rings.Minute, rings.Hour, rings.Day} {
		b := r.current(t)
		b.Total++
		add(b.Names, name)
		add(b.Users, account)
		add(b.Surfaces, surface)
	}
	dirty = true
}

// Endpoint reduces a request path to something worth counting: the service it
// belongs to, not the individual thing it asked for. "/news" and "/news?id=7"
// are both news; a thousand distinct blog post ids would otherwise be a
// thousand rows saying nothing.
//
// Apps keep their slug, because which app is being used is the interesting
// part, and there are few enough of them to name.
func Endpoint(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return "/"
	}
	parts := strings.Split(path, "/")
	if parts[0] == "apps" && len(parts) > 1 {
		return "apps/" + parts[1]
	}
	if parts[0] == "admin" && len(parts) > 1 {
		return "admin/" + parts[1]
	}
	return parts[0]
}

// Skip reports whether a path is noise rather than usage: assets, the image
// cache, health checks. Counting them buries the traffic that means something.
func Skip(path string) bool {
	for _, prefix := range []string{"/img", "/static", "/favicon", "/robots.txt", "/health", "/updates", "/chat/ws"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// current returns the bucket for t, starting a new one and trimming the ring
// when t has moved past the last.
func (r *ring) current(t time.Time) *Bucket {
	at := t.Truncate(r.Step)
	if n := len(r.Buckets); n > 0 && r.Buckets[n-1].At.Equal(at) {
		return &r.Buckets[n-1]
	}
	r.Buckets = append(r.Buckets, Bucket{
		At:       at,
		Names:    map[string]int{},
		Users:    map[string]int{},
		Surfaces: map[string]int{},
	})
	if len(r.Buckets) > r.Keep {
		r.Buckets = append(r.Buckets[:0:0], r.Buckets[len(r.Buckets)-r.Keep:]...)
	}
	return &r.Buckets[len(r.Buckets)-1]
}

func add(m map[string]int, key string) {
	if m == nil {
		return
	}
	if _, ok := m[key]; !ok && len(m) >= maxKeys {
		m[Other]++
		return
	}
	m[key]++
}

func ringFor(res Resolution) *ring {
	switch res {
	case Hour:
		return rings.Hour
	case Day:
		return rings.Day
	default:
		return rings.Minute
	}
}

// Series returns the last n buckets at a resolution, oldest first, with empty
// buckets filled in — a gap in traffic is information, and a chart that skips
// it lies about the shape.
func Series(res Resolution, n int) []Bucket {
	mu.Lock()
	defer mu.Unlock()

	r := ringFor(res)
	if n <= 0 || n > r.Keep {
		n = r.Keep
	}

	byTime := map[time.Time]Bucket{}
	for _, b := range r.Buckets {
		byTime[b.At] = b
	}

	end := now().UTC().Truncate(r.Step)
	out := make([]Bucket, 0, n)
	for i := n - 1; i >= 0; i-- {
		at := end.Add(-time.Duration(i) * r.Step)
		if b, ok := byTime[at]; ok {
			out = append(out, copyBucket(b))
			continue
		}
		out = append(out, Bucket{At: at})
	}
	return out
}

func copyBucket(b Bucket) Bucket {
	out := Bucket{At: b.At, Total: b.Total,
		Names: map[string]int{}, Users: map[string]int{}, Surfaces: map[string]int{}}
	for k, v := range b.Names {
		out.Names[k] = v
	}
	for k, v := range b.Users {
		out.Users[k] = v
	}
	for k, v := range b.Surfaces {
		out.Surfaces[k] = v
	}
	return out
}

// Count is one row of a breakdown.
type Count struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// Dimension selects which breakdown Top returns.
type Dimension string

const (
	ByName    Dimension = "name"
	ByUser    Dimension = "user"
	BySurface Dimension = "surface"
)

// Top returns the busiest keys over the last n buckets at a resolution,
// highest first.
func Top(res Resolution, n int, dim Dimension, limit int) []Count {
	totals := map[string]int{}
	for _, b := range Series(res, n) {
		var m map[string]int
		switch dim {
		case ByUser:
			m = b.Users
		case BySurface:
			m = b.Surfaces
		default:
			m = b.Names
		}
		for k, v := range m {
			totals[k] += v
		}
	}

	out := make([]Count, 0, len(totals))
	for k, v := range totals {
		out = append(out, Count{Key: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key // stable for equal counts
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// TotalOver sums calls over the last n buckets at a resolution.
func TotalOver(res Resolution, n int) int {
	sum := 0
	for _, b := range Series(res, n) {
		sum += b.Total
	}
	return sum
}

// saver persists the counters on a slow cadence. Losing up to a minute of
// counts to a hard kill is an acceptable trade for not writing the file on
// every request.
func saver() {
	for {
		time.Sleep(time.Minute)
		Save()
	}
}

// Save writes the counters if anything has changed since the last write.
func Save() {
	mu.Lock()
	if !dirty {
		mu.Unlock()
		return
	}
	snapshot := store{
		Minute: &ring{Step: rings.Minute.Step, Keep: rings.Minute.Keep, Buckets: append([]Bucket{}, rings.Minute.Buckets...)},
		Hour:   &ring{Step: rings.Hour.Step, Keep: rings.Hour.Keep, Buckets: append([]Bucket{}, rings.Hour.Buckets...)},
		Day:    &ring{Step: rings.Day.Step, Keep: rings.Day.Keep, Buckets: append([]Bucket{}, rings.Day.Buckets...)},
	}
	dirty = false
	mu.Unlock()

	data.SaveJSON(storeKey, snapshot) //nolint:errcheck — counters are not worth failing a request over
}

// SeriesFor is one account's calls over the last n buckets, in the same shape
// as Series so the same chart draws it.
//
// Only Total is filled. The breakdown maps are left nil deliberately: this
// bucket's Names and Surfaces are the whole instance's, and copying them onto
// one account's series would attribute everyone's calls to that account.
//
// Which tools *this* account called is not answerable from these counters, and
// is not made answerable here — that needs a name-per-user key, which is the
// cardinality explosion the package comment refuses. The wallet's ledger
// already answers it for everything that costs money.
func SeriesFor(account string, res Resolution, n int) []Bucket {
	if account == "" {
		return nil
	}
	out := Series(res, n)
	for i := range out {
		out[i].Total = out[i].Users[account]
		out[i].Names, out[i].Users, out[i].Surfaces = nil, nil, nil
	}
	return out
}

// TotalForOver sums one account's calls over the last n buckets.
func TotalForOver(account string, res Resolution, n int) int {
	sum := 0
	for _, b := range SeriesFor(account, res, n) {
		sum += b.Total
	}
	return sum
}
