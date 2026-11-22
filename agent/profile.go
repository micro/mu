package agent

import (
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
	"strings"
)

// profile serves published metadata for guests and non-owners. Owners continue
// into their existing private console. Publishing never grants execution access.
func profile(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/agent/") {
		return false
	}
	name := strings.TrimPrefix(r.URL.Path, "/agent/")
	if name == "" || strings.Contains(name, "/") || reservedName(name) {
		return false
	}
	a, err := namedAgent(name)
	if err != nil {
		http.Error(w, "Agent directory unavailable", http.StatusServiceUnavailable)
		return true
	}
	_, acc := auth.TrySession(r)
	if acc != nil && ((a != nil && acc.ID == a.Owner) || (a == nil && For(acc.ID, name) != nil)) {
		return false
	}
	if a == nil || !a.ProfilePublic {
		app.NotFound(w, r, "Agent not found")
		return true
	}
	w.Header().Set("Cache-Control", "private, no-store")
	body := `<article class="reader-content"><p>` + html.EscapeString(a.Description) + `</p>`
	if acc == nil {
		body += `<div class="form-actions"><a class="btn" href="/login?next=/agent/` + html.EscapeString(Slug(a)) + `">Sign in</a></div>`
	} else {
		body += `<p>This is a public profile. Chat access is managed by the agent's owner.</p>`
	}
	body += `</article>`
	app.Respond(w, r, app.Response{Title: a.Name, Description: a.Description, HTML: body})
	return true
}
