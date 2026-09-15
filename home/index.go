package home

import (
	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/web"
	"net/http"
)

// Index serves Home and the Assistant app, preserving saved conversation links.
func Index(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	auth.SetCSRFCookie(w, r)
	w.Header().Set("Cache-Control", "private, no-store")
	_, acc := auth.TrySession(r)
	state := map[string]any{"account": nil, "csrf": auth.CSRFToken(r)}
	if acc != nil {
		conversation, err := agent.ClientHistoryForRequest(w, r, acc.ID)
		if err != nil {
			app.NotFound(w, r, err.Error())
			return
		}
		state["account"] = map[string]any{"id": acc.ID, "name": acc.Name, "admin": acc.Admin}
		state["conversation"] = conversation
	}
	title := "Home"
	if r.URL.Path == "/assistant" {
		title = "Assistant"
	}
	if web.Page(w, r, title, state) {
		return
	}
	app.RespondJSON(w, state)
}
