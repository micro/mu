package server

import (
	"mu/service/events"
	"net/http"
)

// Keep old settings URLs and posted forms working after moving their owner.
func eventsPageHandler(w http.ResponseWriter, r *http.Request) {
	view, action := r.URL.Query().Get("view"), r.FormValue("action")
	if view == "brief" || view == "research" || action == "brief-schedule" || action == "checkin-schedule" || action == "research-schedule" {
		destination := "/agents?view=scheduled"
		if view == "research" || action == "research-schedule" {
			destination += "#research"
		}
		if action == "checkin-schedule" {
			destination += "#checkin"
		}
		http.Redirect(w, r, destination, http.StatusTemporaryRedirect)
		return
	}
	events.Handler(w, r)
}
