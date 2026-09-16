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
	command := ""
	switch path {
	case "home", "assistant", "agent", "agent/micro":
		if id := r.URL.Query().Get("session"); id != "" {
			http.Redirect(w, r, "/?session="+url.QueryEscape(id), 303)
			return true
		}
		command = "help"
	case "inbox":
		command = "inbox"
		if id := r.URL.Query().Get("id"); id != "" {
			command += " read " + id
		}
	case "account", "account/profile", "account/billing", "account/usage", "wallet":
		command = "account"
	case "work":
		command = "work"
	case "services", "tools", "agents":
		command = "help"
	case "admin":
		command = "admin"
	case "admin/log", "admin/email":
		command = "admin logs"
	case "admin/config":
		command = "admin config list"
	default:
		if strings.HasPrefix(path, "admin/") {
			command = "admin " + strings.TrimPrefix(path, "admin/")
		}
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
