package places

// Recent searches, kept without being asked.
//
// There was a "Save this search" button under every result list, and it is the
// wrong shape for what it does. Somebody who has just searched for a cafe near
// the office does not want to file that; they want it there tomorrow when they
// look again. Asking is asking somebody to predict, before they know whether
// the answer was any good, whether they will want it back — and the answer to
// "was that useful" arrives after the moment the button was in front of them.
//
// So a search is remembered because it happened, which is what /video and /web
// already do. The only interaction left is removing one.
//
// Server-side and per account, unlike those two, which keep theirs in
// localStorage. A places search is not a word: it is a query, a location, a
// radius and a sort order, and it is worth having on the phone you looked it up
// on and the laptop you did not. The store was already here — this changes when
// it is written, not where.

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"

	"github.com/google/uuid"
)

// SavedSearch represents a saved places search
type SavedSearch struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Type      string    `json:"type"` // "search" or "nearby"
	Query     string    `json:"query,omitempty"`
	Location  string    `json:"location,omitempty"`
	Lat       float64   `json:"lat,omitempty"`
	Lon       float64   `json:"lon,omitempty"`
	Radius    int       `json:"radius,omitempty"`
	SortBy    string    `json:"sort_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	savedMu   sync.RWMutex
	savedData = map[string][]SavedSearch{} // userID -> searches
)

func loadSavedSearches() {
	var d map[string][]SavedSearch
	if err := data.LoadJSON("places_saved.json", &d); err == nil {
		savedMu.Lock()
		savedData = d
		savedMu.Unlock()
	}
}

func persistSavedSearches() {
	savedMu.RLock()
	defer savedMu.RUnlock()
	data.SaveJSON("places_saved.json", savedData)
}

func getUserSavedSearches(userID string) []SavedSearch {
	savedMu.RLock()
	defer savedMu.RUnlock()
	src := savedData[userID]
	out := make([]SavedSearch, len(src))
	copy(out, src)
	return out
}

// maxRecentSearches is how many are kept per account.
//
// The same ten /video keeps. It was twenty, from when each one was a deliberate
// save and the list was short because saving was work. A list that fills itself
// fills up, and twenty rows of near-identical searches above the cities is a
// worse page than ten.
const maxRecentSearches = 10

func addUserSavedSearch(userID string, s SavedSearch) {
	savedMu.Lock()
	// The same search again moves to the front rather than appearing twice.
	// Without this a list that records every search is a list of one search,
	// repeated, for anybody who ran it more than once — which is the shape of
	// looking something up.
	searches := []SavedSearch{s}
	for _, old := range savedData[userID] {
		if sameSearch(old, s) {
			continue
		}
		searches = append(searches, old)
	}
	if len(searches) > maxRecentSearches {
		searches = searches[:maxRecentSearches]
	}
	savedData[userID] = searches
	savedMu.Unlock()
	go persistSavedSearches()
}

// sameSearch reports whether two searches would return the same page.
//
// Everything that goes into the request, and nothing that does not: the id and
// the time it was run are what distinguishes two records of one search, which
// is exactly what this has to see past. The label is derived from the rest, so
// comparing it as well would only find disagreements between it and them.
func sameSearch(a, b SavedSearch) bool {
	return a.Type == b.Type &&
		strings.EqualFold(strings.TrimSpace(a.Query), strings.TrimSpace(b.Query)) &&
		strings.EqualFold(strings.TrimSpace(a.Location), strings.TrimSpace(b.Location)) &&
		a.Lat == b.Lat && a.Lon == b.Lon &&
		a.Radius == b.Radius && a.SortBy == b.SortBy
}

// Remember records a search that just ran.
//
// Called from the two handlers that render a result page, and only for a signed
// in person looking at one — an agent calling places_search is not building
// somebody's recent list, and a JSON caller has nowhere to see it.
//
// Silent about failure by design. Nothing here is worth a broken search page:
// if the list cannot be written the results are still the results.
func Remember(userID, searchType, query, location string, lat, lon float64, radius int, sortBy string) {
	if strings.TrimSpace(userID) == "" {
		return
	}
	if searchType != "nearby" {
		searchType = "search"
	}
	addUserSavedSearch(userID, SavedSearch{
		ID:        uuid.New().String(),
		Label:     searchLabel(query, location),
		Type:      searchType,
		Query:     strings.TrimSpace(query),
		Location:  strings.TrimSpace(location),
		Lat:       lat,
		Lon:       lon,
		Radius:    radius,
		SortBy:    sortBy,
		CreatedAt: time.Now(),
	})
}

// searchLabel is what one row reads as.
func searchLabel(query, location string) string {
	query, location = strings.TrimSpace(query), strings.TrimSpace(location)
	label := query
	if label == "" {
		label = "Nearby"
	}
	if location != "" {
		label += " near " + location
	}
	return label
}

func deleteUserSavedSearch(userID, id string) {
	savedMu.Lock()
	searches := savedData[userID]
	for i, s := range searches {
		if s.ID == id {
			savedData[userID] = append(searches[:i], searches[i+1:]...)
			break
		}
	}
	savedMu.Unlock()
	go persistSavedSearches()
}

// handleDeleteSavedSearch handles POST /places/save/delete
func handleDeleteSavedSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	r.ParseForm()
	id := r.Form.Get("id")
	if id != "" {
		deleteUserSavedSearch(acc.ID, id)
	}
	http.Redirect(w, r, "/places", http.StatusSeeOther)
}
