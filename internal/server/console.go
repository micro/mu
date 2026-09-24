package server

import (
	"mu/internal/app"
	"net/http"
	"net/url"
)

// Only the retired assistant alias redirects. Services remain directly usable;
// their handlers enforce the same permissions as before.
func consoleRedirect(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet || app.WantsJSON(r) || r.URL.Path != "/assistant" {
		return false
	}
	q := url.Values{}
	for _, key := range []string{"session", "continue", "agent", "id", "new"} {
		if value := r.URL.Query().Get(key); value != "" {
			q.Set(key, value)
		}
	}
	target := "/agent"
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
	return true
}
