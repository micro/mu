package server

import (
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
)

// Retired browser destinations converge on the command surface. JSON, writes,
// protocol endpoints and files retain their existing handlers and permissions.
func consoleRedirect(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet || app.WantsJSON(r) || r.Header.Get("X-Mu-Transcript") != "" {
		return false
	}
	path := strings.Trim(r.URL.Path, "/")
	// These pages are part of the restored navigation. Let their handlers
	// render them and enforce their own session and administrator checks.
	if path == "home" || strings.HasPrefix(path, "home/") ||
		path == "agent" || strings.HasPrefix(path, "agent/") ||
		path == "inbox" || strings.HasPrefix(path, "inbox/") ||
		path == "admin" || strings.HasPrefix(path, "admin/") ||
		path == "work" || path == "agents" || path == "services" || path == "tools" || path == "apps" ||
		path == "blog" || strings.HasPrefix(path, "blog/") {
		return false
	}
	if _, acc := auth.TrySession(r); acc != nil && acc.Admin {
		for _, spec := range service.Specs() {
			if r.URL.Path == spec.Page || path == spec.Name {
				return false
			}
		}
	}
	command := ""
	switch path {
	case "assistant":
		target := "/agent"
		q := url.Values{}
		for _, key := range []string{"session", "continue", "agent", "id"} {
			if value := r.URL.Query().Get(key); value != "" {
				q.Set(key, value)
			}
		}
		if len(q) > 0 {
			target += "?" + q.Encode()
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
		return true

	case "wallet":
		command = "account"
	default:
		for _, spec := range service.Specs() {
			if path == spec.Name {
				command = spec.Name
				break
			}
		}
	}
	if command == "" {
		return false
	}
	// Only prefill. Visiting a link must never execute a command or spend credits.
	http.Redirect(w, r, "/agent/micro#"+url.PathEscape(command), http.StatusSeeOther)
	return true
}
