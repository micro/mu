package places

// Searching is what puts a search in the list.
//
// It used to be a star under the results, which asks somebody to decide whether
// they will want a search back before they have seen whether the answer was any
// good. /video and /web do not ask, and neither does this now.

import (
	"strings"
	"testing"
)

func reset(t *testing.T) {
	t.Helper()
	savedMu.Lock()
	savedData = map[string][]SavedSearch{}
	savedMu.Unlock()
}

func TestASearchIsRememberedWithoutBeingAsked(t *testing.T) {
	reset(t)
	const who = "somebody"

	Remember(who, "search", "cafe", "Shoreditch", 51.5, -0.08, 1000, "distance")

	got := getUserSavedSearches(who)
	if len(got) != 1 {
		t.Fatalf("recent searches: %d, want 1", len(got))
	}
	if got[0].Label != "cafe near Shoreditch" {
		t.Errorf("label is %q", got[0].Label)
	}
	if got[0].Query != "cafe" || got[0].Radius != 1000 || got[0].Lat != 51.5 {
		t.Errorf("the search was not recorded as it was run: %+v", got[0])
	}
}

// The same search twice is one row, moved to the front. A list that records
// every search and does not do this is a list of one search repeated, for
// anybody who looked something up more than once.
func TestTheSameSearchTwiceIsOneRow(t *testing.T) {
	reset(t)
	const who = "somebody"

	Remember(who, "search", "cafe", "Shoreditch", 51.5, -0.08, 1000, "distance")
	Remember(who, "search", "pharmacy", "Shoreditch", 51.5, -0.08, 1000, "distance")
	Remember(who, "search", "cafe", "Shoreditch", 51.5, -0.08, 1000, "distance")

	got := getUserSavedSearches(who)
	if len(got) != 2 {
		t.Fatalf("recent searches: %d, want 2 — the repeat was kept as a second row", len(got))
	}
	if got[0].Query != "cafe" {
		t.Errorf("the search just run is not at the front: %q", got[0].Query)
	}
}

// Same words, different place, is a different search. Deduping on the query
// alone would lose the one you ran second.
func TestTheSameWordsElsewhereIsADifferentSearch(t *testing.T) {
	reset(t)
	const who = "somebody"

	Remember(who, "search", "cafe", "Shoreditch", 51.5, -0.08, 1000, "distance")
	Remember(who, "search", "cafe", "Lisbon", 38.7, -9.1, 1000, "distance")

	if got := getUserSavedSearches(who); len(got) != 2 {
		t.Fatalf("recent searches: %d, want 2", len(got))
	}
}

// The list fills itself now, so it has to stop.
func TestTheListStopsAtTen(t *testing.T) {
	reset(t)
	const who = "somebody"

	for i := 0; i < maxRecentSearches+5; i++ {
		Remember(who, "search", strings.Repeat("a", i+1), "", 0, 0, 1000, "")
	}
	if got := getUserSavedSearches(who); len(got) != maxRecentSearches {
		t.Errorf("recent searches: %d, want %d", len(got), maxRecentSearches)
	}
}

// Nobody signed in is nobody's list.
func TestAnAnonymousSearchIsNotRecorded(t *testing.T) {
	reset(t)
	Remember("", "search", "cafe", "", 0, 0, 1000, "")
	if got := getUserSavedSearches(""); len(got) != 0 {
		t.Errorf("a search with no account behind it was recorded: %+v", got)
	}
}

// The map is gone, and nothing should be loading Leaflet from a CDN to draw it.
//
// It did not render, and while it was there it pushed the city list below two
// hundred and eighty pixels of empty box on the one page whose job is to get
// you to a city. The tiles and the library both came from third parties on
// every load, which is the other reason not to put it back without meaning to.
func TestNoMapIsDrawn(t *testing.T) {
	initCities()
	for _, page := range []string{
		renderCitiesSection(),
		renderSearchResults("cafe", nil, false, "", 0, 0, "", 1000),
		renderNearbyResults("Lisbon", 38.7, -9.1, 1000, nil),
	} {
		for _, dead := range []string{"leaflet", "map-box", "L.map(", "selectCity"} {
			if strings.Contains(strings.ToLower(page), strings.ToLower(dead)) {
				t.Errorf("%q is still on a places page", dead)
			}
		}
	}
}

// And the cities are links, so they work before any script has run and can be
// opened in a new tab.
func TestCitiesAreLinks(t *testing.T) {
	initCities()
	html := renderCitiesSection()
	if html == "" {
		t.Fatal("no cities rendered, so this test would pass whatever the links were")
	}
	if strings.Contains(html, `href="#"`) || strings.Contains(html, "onclick=") {
		t.Error("a city is still a fake link with a handler on it")
	}
	if !strings.Contains(html, "/places/nearby?lat=") {
		t.Errorf("a city does not link to a nearby search: %.200s", html)
	}
}
