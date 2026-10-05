package work

import (
	"encoding/json"
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/tasks"
	"net/http"
	"strings"
)

func diagnosticReport(t *tasks.Task) string {
	// No prompt or conversation body is added to a report copied for support.
	report := map[string]any{"work_id": t.ID, "status": t.Status, "created": t.Created, "updated": t.Updated, "attempts": t.Attempts, "steps": t.Steps, "outcome": t.Result}
	b, _ := json.MarshalIndent(report, "", "  ")
	return string(b)
}

// AdminHandler looks up an opaque work reference without requiring log searches.
func AdminHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil || !acc.Admin {
		app.Forbidden(w, r, "Operator access required")
		return
	}
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	body := `<form class="search-bar" method="GET" action="/admin/work"><input name="id" aria-label="Work ID" placeholder="Work ID" required><button>Find</button></form>`
	if id != "" {
		found := false
		for _, owner := range auth.AllAccounts() {
			if owner == nil {
				continue
			}
			t, e := tasks.Get(owner.ID, id)
			if e != nil {
				continue
			}
			body += `<h2>` + html.EscapeString(t.Title) + `</h2><pre>` + html.EscapeString(diagnosticReport(t)) + `</pre>`
			found = true
			break
		}
		if !found {
			body += fmt.Sprintf(`<p>No work found for %s.</p>`, html.EscapeString(id))
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	app.Respond(w, r, app.Response{Title: "Work diagnostics", HTML: body})
}
