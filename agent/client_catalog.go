package agent

import (
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
)

// Client discovery exposes presentation only, never prompts, tokens or tool scope.
func clientCatalog(w http.ResponseWriter, r *http.Request) {
	if !api.AuthorizeProduct(w, r, "agent", false) {
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	app.RespondJSON(w, map[string]any{"agents": ClientAgents(acc.ID)})
}

// ClientAgent is the presentation of an accessible conversational agent.
type ClientAgent struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ClientAgents lists owned hosted agents and built-ins, resolving name collisions consistently.
func ClientAgents(owner string) []ClientAgent {
	entries := []ClientAgent{}
	seen := map[string]bool{}
	for _, a := range Agents(owner) {
		if a.Kind != Hosted {
			continue
		}
		slug := Slug(a)
		seen[slug] = true
		entries = append(entries, ClientAgent{a.ID, slug, a.Name, a.Description})
	}
	for _, a := range Builtins() {
		if !seen[a.ID] {
			entries = append(entries, ClientAgent{a.ID, a.ID, a.Name, a.Description})
		}
	}
	return entries
}
