package places

import "testing"

func TestRecentSearchIdentity(t *testing.T) {
	base := SavedSearch{Type: "search", Query: "coffee", Location: "London", Lat: 51.5074, Lon: -0.1278}
	for _, tc := range []struct {
		name   string
		search SavedSearch
		same   bool
	}{
		{"coordinate label", SavedSearch{Query: " COFFEE ", Location: "51.507400,-0.127800"}, true},
		{"different geocoder label", SavedSearch{Query: "coffee", Location: "London, UK", Lat: 51.5074001, Lon: -0.1278}, true},
		{"different place", SavedSearch{Query: "coffee", Lat: 52, Lon: -0.1278}, false},
		{"different query", SavedSearch{Query: "tea", Lat: 51.5074, Lon: -0.1278}, false},
		{"different radius", SavedSearch{Query: "coffee", Lat: 51.5074, Lon: -0.1278, Radius: 5000}, false},
		{"different sort", SavedSearch{Query: "coffee", Lat: 51.5074, Lon: -0.1278, SortBy: "rating"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if sameSearch(base, tc.search) != tc.same {
				t.Fatal("wrong recent search identity")
			}
		})
	}
}

func TestRecentSearchDedupIsAccountScoped(t *testing.T) {
	a := SavedSearch{ID: "new", Query: "coffee", Location: "London", Lat: 51.5, Lon: -0.12}
	b := a
	b.ID = "old"
	b.Location = "51.500000,-0.120000"
	savedMu.Lock()
	savedData["recent-test"] = []SavedSearch{a, b}
	savedData["other-test"] = []SavedSearch{b}
	savedMu.Unlock()
	defer func() {
		savedMu.Lock()
		delete(savedData, "recent-test")
		delete(savedData, "other-test")
		savedMu.Unlock()
	}()
	got := getUserSavedSearches("recent-test")
	if len(got) != 1 || got[0].ID != "new" {
		t.Fatalf("duplicates remain: %#v", got)
	}
	if len(getUserSavedSearches("other-test")) != 1 {
		t.Fatal("another account was affected")
	}
}
