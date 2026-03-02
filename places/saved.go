package places

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/auth"
	"mu/data"

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

func addUserSavedSearch(userID string, s SavedSearch) {
	savedMu.Lock()
	// Prepend new search; limit to 20 per user
	searches := append([]SavedSearch{s}, savedData[userID]...)
	if len(searches) > 20 {
		searches = searches[:20]
	}
	savedData[userID] = searches
	savedMu.Unlock()
	go persistSavedSearches()
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

// handleSaveSearch handles POST /places/save
func handleSaveSearch(w http.ResponseWriter, r *http.Request) {
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

	searchType := r.Form.Get("type")
	if searchType != "nearby" {
		searchType = "search"
	}

	query := strings.TrimSpace(r.Form.Get("q"))
	location := strings.TrimSpace(r.Form.Get("near"))
	latStr := r.Form.Get("near_lat")
	lonStr := r.Form.Get("near_lon")
	radius, _ := strconv.Atoi(r.Form.Get("radius"))
	sortBy := r.Form.Get("sort")

	var lat, lon float64
	if latStr != "" && lonStr != "" {
		lat, _ = strconv.ParseFloat(latStr, 64)
		lon, _ = strconv.ParseFloat(lonStr, 64)
	}

	label := query
	if label == "" {
		label = "Nearby"
	}
	if location != "" {
		label += " near " + location
	}

	s := SavedSearch{
		ID:        uuid.New().String(),
		Label:     label,
		Type:      searchType,
		Query:     query,
		Location:  location,
		Lat:       lat,
		Lon:       lon,
		Radius:    radius,
		SortBy:    sortBy,
		CreatedAt: time.Now(),
	}
	addUserSavedSearch(acc.ID, s)

	http.Redirect(w, r, "/places", http.StatusSeeOther)
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
