package data

// What is in the archive, by kind.
//
// Six services write to this index — news, video, markets, blog, prayer,
// social — and between them they have put a large number of things in it over a
// long time. Nothing could say how many, or of what: the only readers were
// filtered to one type each, so the store was a thing you could ask about news
// and could not ask about itself.
//
// That is the question a page has to answer before it can be a page, and it is
// the question anybody deciding what to keep has to answer first.

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Kind is one type of thing in the archive and how much of it there is.
type Kind struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Kinds is what has been archived, largest first.
//
// Public entries only. A per-account count would be a different question and a
// worse one to answer here — an entry with an owner is somebody's private
// record, and how many of them exist is not archive statistics.
//
// # Cached, because it is a whole-table question asked on every page load
//
// GROUP BY over every public row, and /archive calls it on every request
// including the empty one. Measured on a copy of a real index grown to 126,208
// rows: 152ms, for five numbers on five chips.
//
// A count is also the thing on that page least harmed by being slightly stale.
// Nobody navigates by whether it says 658 or 659, and the alternative — exact
// on every request — is paying a full grouping to be precise about a number
// that changes while you are reading it.
func Kinds() []Kind {
	kindsMu.RLock()
	if time.Since(kindsAt) < kindsFor && kindsAt.After(time.Time{}) {
		out := append([]Kind(nil), kindsCache...)
		kindsMu.RUnlock()
		return out
	}
	kindsMu.RUnlock()

	out := countKinds()

	kindsMu.Lock()
	kindsCache, kindsAt = out, time.Now()
	kindsMu.Unlock()
	return append([]Kind(nil), out...)
}

// kindsFor is how long a count is allowed to be stale.
//
// Long enough that a burst of readers costs one grouping, short enough that
// somebody watching a service fill the archive sees it move.
const kindsFor = 30 * time.Second

var (
	kindsMu    sync.RWMutex
	kindsCache []Kind
	kindsAt    time.Time
)

// InvalidateKinds drops the cached counts, for a caller that has just changed
// what is in the index enough to care — a bulk delete, a migration.
func InvalidateKinds() {
	kindsMu.Lock()
	kindsAt = time.Time{}
	kindsMu.Unlock()
}

func countKinds() []Kind {
	counts := map[string]int{}

	if UseSQLite {
		db, err := getDB()
		if err != nil {
			return nil
		}
		rows, err := db.Query(`SELECT type, COUNT(*) FROM index_entries WHERE owner = '' GROUP BY type`)
		if err != nil {
			return nil
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var n int
			if err := rows.Scan(&name, &n); err != nil {
				continue
			}
			counts[name] = n
		}
	} else {
		indexMutex.RLock()
		for _, e := range index {
			if e.Owner != "" {
				continue
			}
			counts[e.Type]++
		}
		indexMutex.RUnlock()
	}

	return sorted(counts)
}

// sorted is a count map as the chips want it: largest first, ties by name.
func sorted(counts map[string]int) []Kind {
	out := make([]Kind, 0, len(counts))
	for name, n := range counts {
		if name == "" {
			continue
		}
		out = append(out, Kind{Name: name, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// KindsMatching is how many of each kind a query matches.
//
// The chips on /archive were drawn from Kinds, which counts the whole archive —
// so searching "bitcoin" put "market 658" beside a list of bitcoin results and
// invited the reading that 658 of them were market entries. The number was true
// about the archive and false about everything else on the page.
//
// Through FTS5 rather than the LIKE path: this is a count, and counting is the
// one thing a scan is worst at — it cannot stop early. FTS5 answered a search
// over 126,208 rows in 0.4ms, and grouping its matches is the same lookup with
// a GROUP BY on the end. Where FTS5 finds nothing the answer is no chips, which
// is correct: there are no results to break down.
//
// Owner-scoped like every other reader here — public entries plus the caller's
// own, and never anybody else's.
func KindsMatching(query string, opts ...SearchOption) []Kind {
	options := &SearchOptions{}
	for _, opt := range opts {
		opt(options)
	}

	counts := map[string]int{}
	if UseSQLite {
		db, err := getDB()
		if err != nil {
			return nil
		}
		words := strings.Fields(strings.ToLower(query))
		ftsQuery := buildFTS5Query(words)
		if ftsQuery == "" {
			return nil
		}
		rows, err := db.Query(`
			SELECT e.type, COUNT(*)
			FROM index_fts f JOIN index_entries e ON e.rowid = f.rowid
			WHERE index_fts MATCH ? AND (e.owner = '' OR e.owner = ?)
			GROUP BY e.type`, ftsQuery, options.Owner)
		if err != nil {
			return nil
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var n int
			if err := rows.Scan(&name, &n); err == nil {
				counts[name] = n
			}
		}
	} else {
		// The in-memory index has no FTS, so this is the same matcher Search
		// uses — which is a scan, and is what that backend is for the whole way
		// down. It is the development and small-instance path.
		words := strings.Fields(strings.ToLower(query))
		indexMutex.RLock()
		for _, e := range index {
			if e.Owner != "" && e.Owner != options.Owner {
				continue
			}
			if scoreMatch(e, words) > 0 {
				counts[e.Type]++
			}
		}
		indexMutex.RUnlock()
	}
	return sorted(counts)
}
