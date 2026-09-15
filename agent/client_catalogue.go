package agent

import (
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
)

// CatalogueHandler serves the browser's public built-ins and owned roster.
func CatalogueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	if _, acc := auth.TrySession(r); acc != nil {
		AgentsHandler(w, r)
		return
	}
	app.RespondJSON(w, map[string]any{"agents": []any{}, "builtins": Builtins()})
}
