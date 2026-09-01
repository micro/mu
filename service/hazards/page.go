package hazards

// /hazards, because every other service answers at its own name.
//
// This was the one service in the catalogue with no top-level route: /news,
// /weather, /markets, /prayer and thirty others all answer, and /hazards was a
// 404. Its tools were fine — hazards_quakes, hazards_alerts and hazards_floods
// all serve over /api/v1 and MCP — and /services/hazards, the derived page every
// service gets, was there too. What was missing was the name itself.
//
// It went missing as a side effect rather than as a decision. This service had
// a hand-drawn page — a magnitude picker, a period picker, a table and a
// stylesheet, all of it a form over one tool with one argument — and deleting
// it was right: the derived page is the card plus every method with its
// arguments, which is strictly more. But the route went with the page, and
// nothing put the name back.
//
// So: JSON for anything asking for JSON, and a person goes to the derived page.
// Not a second hand-drawn page. The reason the last one was deleted has not
// changed, and a service whose answer is a list of recent events has nothing
// particular to show that the card does not already show.

import (
	"net/http"

	"mu/internal/app"
)

// defaultMagnitude and defaultPeriod are what the card asks for, so /hazards
// and the card on Home answer the same question rather than two similar ones.
const (
	defaultMagnitude = 4.5
	defaultPeriod    = "day"
)

// Handler serves /hazards.
func Handler(w http.ResponseWriter, r *http.Request) {
	if app.WantsJSON(r) {
		quakes, err := recent(defaultMagnitude, defaultPeriod, 0, 0, 0)
		if err != nil {
			app.RespondError(w, http.StatusBadGateway,
				"the earthquake feed did not answer: "+err.Error())
			return
		}
		app.RespondJSON(w, map[string]interface{}{"quakes": quakes})
		return
	}

	// The derived page, at its own URL rather than rendered here, so what a
	// person is looking at and what the address bar says are the same thing.
	http.Redirect(w, r, "/services/hazards", http.StatusFound)
}
