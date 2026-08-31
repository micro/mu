package transit

// Everywhere that is not London.
//
// TfL answers live — it knows the bus is four minutes away because the bus said
// so — and it answers for one city. GTFS answers from the timetable, and it
// answers for any city whose agency publishes one, which is most of them.
//
// So the two are not alternatives, they are layers. A question about London
// goes to TfL because a live answer beats a scheduled one. A question about
// anywhere else goes to the timetable. The caller is told which it got, because
// "due at 23:03" and "four minutes away" are different promises, but it does
// not have to ask for one or the other — that is the uniform surface the whole
// repository is arguing for, and this is the clearest case of it we have.
//
// Feeds are named in the TRANSIT_FEEDS setting, as ids or as words to look up
// in the catalogue: "reading buses, bart, vbb". Nothing is downloaded that an
// operator did not ask for, because a feed is tens of megabytes and a query
// should never trigger one.

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/dir"
	"mu/internal/gtfs"
	"mu/internal/settings"
)

var (
	storeOnce sync.Once
	store     *gtfs.Store
)

// feedStore opens the on-disk feeds, once.
func feedStore() *gtfs.Store {
	storeOnce.Do(func() {
		feeds := filepath.Join(dir.Data(), "gtfs")
		if err := os.MkdirAll(feeds, 0o755); err != nil {
			app.Log("transit", "could not open the timetable directory: %v", err)
		}
		store = gtfs.NewStore(feeds)
		if loaded := store.Loaded(); len(loaded) > 0 {
			app.Log("transit", "timetables loaded: %s", strings.Join(loaded, ", "))
		}
	})
	return store
}

// configuredFeeds is what the operator asked for.
func configuredFeeds() []string {
	raw := settings.Get("TRANSIT_FEEDS")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// StartFeeds keeps the configured timetables current.
//
// Once a day, because agencies republish when a timetable changes and nobody
// changes one hourly. The check itself is conditional, so a feed that has not
// moved costs a 304 and no bytes at all — the 75MB is only spent when Berlin
// has actually rewritten its timetable.
func StartFeeds() {
	if len(configuredFeeds()) == 0 {
		return
	}
	go func() {
		// A little after boot rather than during it: nothing here is needed to
		// serve the first request, and an instance coming up should come up.
		time.Sleep(30 * time.Second)
		for {
			refreshFeeds()
			time.Sleep(gtfs.RefreshInterval)
		}
	}()
}

// refreshFeeds updates each configured feed in turn.
func refreshFeeds() {
	s := feedStore()
	for _, want := range configuredFeeds() {
		feed, ok := gtfs.FindFeed(want)
		if !ok {
			app.Log("transit", "no feed in the catalogue matches %q", want)
			continue
		}
		// The agency first, the catalogue's mirror second. Mirrors lag — one
		// was serving a timetable two months out of date — so a mirror is what
		// you fall back to when an agency is unreachable, not what you trust.
		changed, err := s.Refresh(feed.ID, feed.Direct, feed.Mirror)
		switch {
		case err != nil:
			// Whatever was loaded before is still loaded. That is the point of
			// the swap, and it is why this is a log line and not an outage.
			app.Log("transit", "could not refresh %s (%s): %v", feed.ID, feed.Provider, err)
		case changed:
			app.Log("transit", "timetable updated: %s (%s)", feed.ID, feed.Provider)
		}
	}
}

// scheduled is a departure from the timetable, rendered for a caller.
type scheduled struct {
	Feed    string
	Stop    string
	Expired bool
	Until   time.Time
	Next    []gtfs.Next
}

// timetableNear finds the feed covering a point and the stops around it.
func timetableNear(lat, lon float64, limit int) (*gtfs.Index, []gtfs.Stop) {
	idx, km := feedStore().NearestFeed(lat, lon)
	if idx == nil {
		return nil, nil
	}
	// A feed for another country technically has a nearest stop. Fifty
	// kilometres is far enough to be somewhere else, and answering with it
	// would be worse than answering with nothing.
	if km > 50 {
		return nil, nil
	}
	return idx, idx.Near(lat, lon, limit)
}

// timetableAt answers "what is next here" from whichever loaded feed knows the
// stop.
func timetableAt(query string, limit int) (*scheduled, bool) {
	for _, idx := range feedStore().All() {
		stopIdx, ok := idx.Find(query)
		if !ok {
			continue
		}
		now := time.Now()
		out := &scheduled{Feed: idx.Meta.FeedID, Stop: idx.Meta.Stops[stopIdx].Name}
		if expired, end := idx.Meta.Expired(now); expired {
			// Say the timetable ran out rather than that no buses are due. One
			// is a fact about the world and the other is a fact about us.
			out.Expired, out.Until = true, end
			return out, true
		}
		next, err := idx.NextAt(stopIdx, now, limit)
		if err != nil {
			continue
		}
		out.Next = next
		return out, true
	}
	return nil, false
}

// FeedSummary describes what this instance has timetables for.
func FeedSummary() []string {
	var out []string
	for _, idx := range feedStore().All() {
		m := idx.Meta
		line := m.FeedID + " — " + itoa(len(m.Stops)) + " stops"
		if expired, end := m.Expired(time.Now()); expired {
			line += ", timetable ran out " + end.Format("2 Jan 2006")
		} else if m.CoversTo > 0 {
			line += ", through " + dateWord(m.CoversTo)
		}
		out = append(out, line)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// dateWord renders a yyyymmdd the way somebody would say it.
func dateWord(d int) string {
	t := time.Date(d/10000, time.Month(d/100%100), d%100, 0, 0, 0, 0, time.UTC)
	return t.Format("2 Jan 2006")
}

// renderScheduled writes timetable departures the way a departure board does.
func renderScheduled(s *scheduled) string {
	if s.Expired {
		return s.Stop + " — this timetable ran out on " + s.Until.Format("2 Jan 2006") +
			", so there is nothing to report. The feed needs updating."
	}
	if len(s.Next) == 0 {
		return "Nothing more due at " + s.Stop + " today."
	}

	var b strings.Builder
	b.WriteString(s.Stop + "\n")
	for _, n := range s.Next {
		line := n.Route
		if line == "" {
			line = gtfs.KindWord(n.Type)
		}
		b.WriteString("  " + line)
		if n.Headsign != "" {
			b.WriteString(" to " + n.Headsign)
		}
		b.WriteString(" — " + n.When.Format("15:04"))
		if n.In > 0 && n.In < 90*time.Minute {
			b.WriteString(" (" + shortDur(n.In) + ")")
		}
		b.WriteString("\n")
	}
	// Said once, at the end, because every line being "scheduled" is noise but
	// the caller still must not read this as live tracking.
	b.WriteString("Scheduled times from the published timetable, not live tracking.")
	return b.String()
}

// shortDur renders a wait the way somebody would say it.
func shortDur(d time.Duration) string {
	m := int(d.Minutes())
	if m < 1 {
		return "due"
	}
	if m < 60 {
		return itoa(m) + " min"
	}
	h := m / 60
	rem := m % 60
	if rem == 0 {
		return itoa(h) + "h"
	}
	return itoa(h) + "h " + itoa(rem) + "m"
}
