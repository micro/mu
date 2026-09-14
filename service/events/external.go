package events

import "time"

// External preserves the events_list JSON schema for existing clients.
// External calendar integrations have been removed; this list is always empty.
type External struct {
	UID      string    `json:"-"` // iCalendar identity, never an authorization grant.
	URL      string    `json:"url,omitempty"`
	Title    string    `json:"title"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Location string    `json:"location"`
	AllDay   bool      `json:"all_day"`
	// Source names where it came from, for a UI that must never imply Mu
	// scheduled something it did not.
	Source string `json:"source"`
}
