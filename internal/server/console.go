package server

import (
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
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
	if path == "inbox" || strings.HasPrefix(path, "inbox/") ||
		path == "admin" || strings.HasPrefix(path, "admin/") ||
		path == "agents" || path == "services" || path == "tools" || path == "apps" {
		return false
	}
	command := ""
	switch path {
	case "home", "assistant", "agent", "agent/micro":
		if id := r.URL.Query().Get("session"); id != "" {
			http.Redirect(w, r, "/?session="+url.QueryEscape(id), 303)
			return true
		}
		command = "help"
	case "wallet":
		command = "account"
	case "work":
		command = "work"
		if id := r.URL.Query().Get("id"); id != "" {
			command += " get " + id
		}
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
	http.Redirect(w, r, "/#"+url.PathEscape(command), http.StatusSeeOther)
	return true
}
