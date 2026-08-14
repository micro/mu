package transit

// The shortlist has to be true.
//
// A table of feed names is only useful if the names still find the feeds they
// were written for. Agencies get renamed and merged, and a name that quietly
// stops matching turns a recommendation into a log line nobody reads.

import (
	"strings"
	"testing"

	"mu/internal/gtfs"
)

func allKnown() []Known {
	return append(append([]Known{}, KnownFeeds...), Expired...)
}

func TestTheShortlistIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range allKnown() {
		switch {
		case strings.TrimSpace(k.Query) == "":
			t.Errorf("an entry has no query: %+v", k)
		case strings.ToLower(k.Query) != k.Query:
			t.Errorf("%q is not lowercase — matching is case-insensitive but the table is read by people", k.Query)
		case len(k.Country) != 2:
			t.Errorf("%q has country %q, want two letters", k.Query, k.Country)
		case k.MB <= 0:
			t.Errorf("%q has no measured size — every entry here was downloaded", k.Query)
		case strings.TrimSpace(k.Place) == "":
			t.Errorf("%q has no place", k.Query)
		}
		if seen[k.Query] {
			t.Errorf("%q is listed twice", k.Query)
		}
		seen[k.Query] = true
	}
}

func TestNothingIsBothRecommendedAndExpired(t *testing.T) {
	rec := map[string]bool{}
	for _, k := range KnownFeeds {
		rec[k.Query] = true
	}
	for _, k := range Expired {
		if rec[k.Query] {
			t.Errorf("%q is on both lists", k.Query)
		}
	}
}

// TestEveryShortlistedNameStillFindsItsFeed needs the catalogue, so it is
// skipped in short mode with the rest of the network-touching tests.
func TestEveryShortlistedNameStillFindsItsFeed(t *testing.T) {
	if testing.Short() {
		t.Skip("fetches the feed catalogue")
	}
	for _, k := range allKnown() {
		f, ok := gtfs.FindFeed(k.Query)
		if !ok {
			t.Errorf("%q no longer matches anything in the catalogue", k.Query)
			continue
		}
		// The country is the cheap check that catches the real failure, which
		// is a name landing on a different operator on another continent —
		// "metrobus" found Metrobus Transit of Newfoundland before the one
		// that runs buses around Gatwick.
		if f.Country != k.Country {
			t.Errorf("%q is listed as %s but now resolves to %s (%s, %s)",
				k.Query, k.Country, f.Country, f.Provider, f.Place)
		}
	}
}
