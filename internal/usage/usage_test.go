package usage

import (
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-usage-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// clock replaces time for a test and returns a function to move it forward.
func clock(t *testing.T, start time.Time) func(time.Duration) {
	t.Helper()
	at := start
	prev := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = prev; blank() })
	return func(d time.Duration) { at = at.Add(d) }
}

// blank empties the counters between tests.
func blank() {
	mu.Lock()
	rings = newStore()
	dirty = false
	mu.Unlock()
}

func TestRecordCountsByEveryDimension(t *testing.T) {
	blank()
	clock(t, time.Date(2026, 8, 3, 12, 30, 0, 0, time.UTC))

	Record("web", "news", "alice")
	Record("web", "news", "bob")
	Record("mcp", "web_search", "alice")

	if got := TotalOver(Minute, 5); got != 3 {
		t.Errorf("total = %d, want 3", got)
	}

	names := Top(Minute, 5, ByName, 10)
	if len(names) != 2 || names[0].Key != "news" || names[0].Count != 2 {
		t.Errorf("by name = %+v", names)
	}
	users := Top(Minute, 5, ByUser, 10)
	if users[0].Key != "alice" || users[0].Count != 2 {
		t.Errorf("by user = %+v", users)
	}
	surfaces := Top(Minute, 5, BySurface, 10)
	if len(surfaces) != 2 {
		t.Errorf("by surface = %+v", surfaces)
	}
}

// The whole point is a shape over time, so a call at 12:30 and one at 12:31
// must not land in the same bar.
func TestCountsLandInTheRightBucket(t *testing.T) {
	blank()
	tick := clock(t, time.Date(2026, 8, 3, 12, 30, 0, 0, time.UTC))

	Record("web", "news", "alice")
	tick(time.Minute)
	Record("web", "news", "alice")
	Record("web", "news", "alice")

	series := Series(Minute, 2)
	if len(series) != 2 {
		t.Fatalf("got %d buckets", len(series))
	}
	if series[0].Total != 1 || series[1].Total != 2 {
		t.Errorf("buckets = %d, %d; want 1, 2", series[0].Total, series[1].Total)
	}

	// The same three calls are one bar at hour resolution.
	if hours := Series(Hour, 1); len(hours) != 1 || hours[0].Total != 3 {
		t.Errorf("hour bucket = %+v, want one bucket of 3", hours)
	}
}

// A deploy is a restart, and Mu deploys on every push. Counters that reset on
// restart would only ever show the last few minutes.
func TestCountersSurviveARestart(t *testing.T) {
	blank()
	clock(t, time.Date(2026, 8, 3, 12, 30, 0, 0, time.UTC))

	for i := 0; i < 7; i++ {
		Record("mcp", "news_list", "alice")
	}
	Save()

	// Restart: the process forgets everything and reads the file back.
	blank()
	if TotalOver(Day, 1) != 0 {
		t.Fatal("the counters were not actually cleared")
	}
	restore()

	if got := TotalOver(Day, 1); got != 7 {
		t.Errorf("after a restart the day shows %d calls, want 7", got)
	}
	if got := TotalOver(Minute, 5); got != 7 {
		t.Errorf("after a restart the minute shows %d calls, want 7", got)
	}
	names := Top(Day, 1, ByName, 5)
	if len(names) != 1 || names[0].Key != "news_list" || names[0].Count != 7 {
		t.Errorf("the breakdown did not survive: %+v", names)
	}
}

// Save is skipped when nothing changed, so an idle instance is not rewriting
// the same file every minute.
func TestSaveOnlyWritesWhenSomethingChanged(t *testing.T) {
	blank()
	clock(t, time.Date(2026, 8, 3, 12, 30, 0, 0, time.UTC))

	Record("web", "news", "alice")
	Save()

	path := os.Getenv("HOME") + "/.mu/data/" + storeKey
	first, err := os.Stat(path)
	if err != nil {
		t.Fatalf("nothing was written: %v", err)
	}

	Save() // no new counts
	second, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !first.ModTime().Equal(second.ModTime()) {
		t.Error("an idle Save rewrote the file")
	}
}

// Old buckets fall off the end rather than accumulating forever.
func TestRingsAreBounded(t *testing.T) {
	blank()
	tick := clock(t, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC))

	for i := 0; i < keepMinutes+30; i++ {
		Record("web", "news", "alice")
		tick(time.Minute)
	}

	mu.Lock()
	n := len(rings.Minute.Buckets)
	mu.Unlock()
	if n != keepMinutes {
		t.Errorf("minute ring holds %d buckets, want %d", n, keepMinutes)
	}
}

// One visitor hitting a thousand distinct paths must not grow the file
// forever.
func TestBreakdownsAreCapped(t *testing.T) {
	blank()
	clock(t, time.Date(2026, 8, 3, 12, 30, 0, 0, time.UTC))

	for i := 0; i < maxKeys*2; i++ {
		Record("web", "path"+string(rune('a'+i%26))+string(rune('a'+i/26)), "crawler")
	}

	mu.Lock()
	n := len(rings.Minute.Buckets[0].Names)
	mu.Unlock()
	if n > maxKeys+1 { // +1 for "other" itself
		t.Errorf("a bucket holds %d keys, want at most %d", n, maxKeys+1)
	}

	var other int
	for _, c := range Top(Minute, 1, ByName, 0) {
		if c.Key == Other {
			other = c.Count
		}
	}
	if other == 0 {
		t.Error("nothing was folded into other; the cap is not doing anything")
	}
}

// The chart is a shape over time, so a quiet minute has to be a gap rather than
// a missing bar that makes the busy ones look adjacent.
func TestQuietPeriodsAreZerosNotGaps(t *testing.T) {
	blank()
	tick := clock(t, time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC))

	Record("web", "news", "alice")
	tick(5 * time.Minute)
	Record("web", "news", "alice")

	series := Series(Minute, 6)
	if len(series) != 6 {
		t.Fatalf("got %d points, want 6", len(series))
	}
	if series[0].Total != 1 || series[5].Total != 1 {
		t.Errorf("the two recorded minutes are not at the ends: %v", totals(series))
	}
	for i := 1; i < 5; i++ {
		if series[i].Total != 0 {
			t.Errorf("minute %d should be empty: %v", i, totals(series))
		}
	}
}

func totals(bs []Bucket) []int {
	out := make([]int, len(bs))
	for i, b := range bs {
		out[i] = b.Total
	}
	return out
}

func TestEndpointNames(t *testing.T) {
	cases := map[string]string{
		"/news":                  "news",
		"/news/":                 "news",
		"/":                      "/",
		"/blog/post":             "blog",
		"/apps/pomodoro-timer":   "apps/pomodoro-timer",
		"/apps":                  "apps",
		"/admin/traffic":         "admin/traffic",
		"/files/abc-123":         "files",
		"/images/file/xyz":       "images",
		"/.well-known/something": ".well-known",
	}
	for in, want := range cases {
		if got := Endpoint(in); got != want {
			t.Errorf("Endpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSkipsNoise(t *testing.T) {
	for _, p := range []string{"/img?u=x", "/favicon.ico", "/updates", "/chat/ws"} {
		if !Skipped(p) {
			t.Errorf("%s should not be counted as usage", p)
		}
	}
	for _, p := range []string{"/news", "/mail", "/apps/timer", "/"} {
		if Skipped(p) {
			t.Errorf("%s is real usage and must be counted", p)
		}
	}
}

// A caller's own usage is their bucket's count for their account, and nobody
// else's.
func TestSeriesForIsOneAccountsCallsOnly(t *testing.T) {
	blank()
	clock(t, time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC))

	Record("mcp", "news_list", "alice")
	Record("mcp", "news_list", "alice")
	Record("web", "mail", "bob")

	got := SeriesFor("alice", Minute, 1)
	if len(got) != 1 || got[0].Total != 2 {
		t.Fatalf("alice's series = %+v, want one bucket of 2", got)
	}
	// The instance-wide breakdowns must not be copied onto one account's
	// series: they are everyone's calls, and showing them here would credit
	// bob's traffic to alice.
	if got[0].Names != nil || got[0].Users != nil || got[0].Surfaces != nil {
		t.Errorf("a per-account bucket carried instance-wide breakdowns: %+v", got[0])
	}

	if n := TotalForOver("bob", Minute, 1); n != 1 {
		t.Errorf("bob's total = %d, want 1", n)
	}
	if n := TotalForOver("nobody", Minute, 1); n != 0 {
		t.Errorf("an account with no calls totalled %d", n)
	}
	if got := SeriesFor("", Minute, 1); got != nil {
		t.Errorf("a signed-out caller got a series: %+v", got)
	}
}
