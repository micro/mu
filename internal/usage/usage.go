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

	// maxPairs bounds the who-called-what index, which is the one map whose
	// keys multiply. Larger than maxKeys because a cap that bites routinely
	// would make every busy caller's breakdown a guess; bounded because this
	// is held per bucket and there are hundreds of buckets.
	maxPairs = 1000

	// Other is where counts land once a bucket is full.
	Other = "other"

	storeKey = "usage/counters.json"
)

// Bucket is one interval's counts.
//
// Pairs is the one cross-index: who called what. The other three maps are
// independent tallies, which is why "asim made 400 calls" and "news_list was
// called 900 times" could both be on the page with no way to ask what asim
// called — the association was never stored. An admin looking at a busy caller
// wants exactly that question answered, and no combination of the other three
// answers it.
//
// It costs the pairs actually observed rather than users times names: a minute
// bucket holds the handful of people who called something in that minute
// crossed with what they touched, not the cross product of the roster and the
// catalogue. And it goes through add() like the rest, so maxKeys caps it and
// the tail lands in Other.
type Bucket struct {
	At       time.Time      `json:"at"`
	Total    int            `json:"total"`
	Names    map[string]int `json:"names,omitempty"`
	Users    map[string]int `json:"users,omitempty"`
	Surfaces map[string]int `json:"surfaces,omitempty"`
	Pairs    map[string]int `json:"pairs,omitempty"`
}

// pairKey is one caller and one thing they called.
//
// NUL-joined because neither half is allowed to contain one and both halves
// can contain anything else: a tool name has underscores, an endpoint can carry
// an app slug, and an account id is whatever signup allowed. Any printable
// separator is a separator one of them could contain.
func pairKey(account, name string) string { return account + "\x00" + name }

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
//   - surface is how it arrived: "web", "mcp", "api", "agent", "cli", "app".
//     "mcp" and "api" are the two tool doors, counted here per tool rather than
//     per path — see internal/server/serve.go.
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
		// A bucket restored from a file written before Pairs existed has a nil
		// map here, and addPair returns on nil rather than panicking. That
		// window is then counted in Users and not in Pairs, which is what
		// TopFor reports as unattributed rather than quietly leaving out.
		addPair(b.Pairs, account, name)
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

// Skipped reports whether a path is noise rather than usage: assets, the image
// cache, health checks. Counting them buries the traffic that means something.
func Skipped(path string) bool {
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
		Pairs:    map[string]int{},
	})
	if len(r.Buckets) > r.Keep {
		r.Buckets = append(r.Buckets[:0:0], r.Buckets[len(r.Buckets)-r.Keep:]...)
	}
	return &r.Buckets[len(r.Buckets)-1]
}

// addPair records who called what, keeping the caller when it has to drop
// something.
//
// Pairs saturates long before the other three maps do: its cardinality is the
// callers seen crossed with what they touched, so 200 keys is reached by a
// handful of busy accounts while Users is nowhere near 200 callers. Going
// through add() meant the overflow landed on the bare "other" key, which
// carries no caller — so those calls vanished from every drill-down while
// still counting in the Callers table, and a row saying 400 opened onto rows
// summing to less with nothing saying why.
//
// The caller is the half worth keeping. It is the lower-cardinality half and
// the one both tables agree on, so overflow goes to <caller>|other: the
// breakdown still adds up, and what it lost is the name of the tool rather
// than the fact that the call happened.
func addPair(m map[string]int, account, name string) {
	if m == nil {
		return
	}
	k := pairKey(account, name)
	if _, seen := m[k]; seen {
		m[k]++
		return
	}
	if len(m) < maxPairs {
		m[k]++
		return
	}
	// Full. Keep the caller, lose the name. Bounded by the number of distinct
	// callers, which is why there is headroom above maxPairs rather than a
	// second unbounded map.
	o := pairKey(account, Other)
	if _, seen := m[o]; seen {
		m[o]++
		return
	}
	if len(m) < maxPairs+maxKeys {
		m[o]++
		return
	}
	m[Other]++
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
		Names: map[string]int{}, Users: map[string]int{}, Surfaces: map[string]int{},
		Pairs: map[string]int{}}
	for k, v := range b.Pairs {
		out.Pairs[k] = v
	}
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

// TopFor is what one caller called, over the same window Top uses, with
// whatever it could not account for.
//
// unattributed is that caller's total in the Callers table minus what the
// breakdown adds up to. It is normally zero. It is not zero in two cases, and
// both are honest rather than broken:
//
//   - Buckets recorded before this index existed. They have counts in Users and
//     a nil Pairs map, so the window can reach back past the deploy that added
//     the breakdown.
//   - Overflow. addPair keeps the caller and drops the tool name past maxPairs,
//     which lands in <caller>|other and is returned as an ordinary row; past
//     that headroom it loses the caller too, and lands here.
//
// Returned rather than absorbed because the alternative is what shipped first:
// a row saying 400 opening onto rows summing to less, with nothing on the page
// saying why. A number that does not add up is a bug report; a number that says
// what it could not see is a measurement.
func TopFor(res Resolution, n int, account string, limit int) (rows []Count, unattributed int) {
	if account == "" {
		return nil, 0
	}
	prefix := account + "\x00"
	totals := map[string]int{}
	var counted, total int
	for _, b := range Series(res, n) {
		for k, v := range b.Pairs {
			if strings.HasPrefix(k, prefix) {
				totals[k[len(prefix):]] += v
				counted += v
			}
		}
		total += b.Users[account]
	}
	if d := total - counted; d > 0 {
		unattributed = d
	}
	return ranked(totals, limit), unattributed
}

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

	return ranked(totals, limit)
}

// ranked turns a tally into rows, busiest first, capped.
func ranked(totals map[string]int, limit int) []Count {
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
