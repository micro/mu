package server

import (
	"net/http"
	"net/url"
	"strings"

	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/origin"
)

// Account pages belong to the primary origin. Start authentication there so
// OAuth state, passkeys and session cookies all use the same host. This does
// not redirect protocol requests or forward credentials between origins.
func redirectHostAccount(w http.ResponseWriter, r *http.Request) bool {
	if !origin.IsX402Host(r) || (r.Method != http.MethodGet && r.Method != http.MethodHead) || app.WantsJSON(r) || r.Header.Get("Authorization") != "" || r.Header.Get(api.TokenHeader) != "" {
		return false
	}
	path := r.URL.Path
	switch {
	case path == "/login", path == "/signup", path == "/logout", path == "/account", strings.HasPrefix(path, "/account/"), strings.HasPrefix(path, "/oauth2/"):
	default:
		return false
	}
	primary, err := url.Parse(origin.Self())
	if err != nil || primary.Host == "" || (primary.Scheme != "https" && primary.Scheme != "http") {
		return false
	}
	current, err := url.Parse(origin.URL(r))
	if err != nil || strings.EqualFold(primary.Host, current.Host) {
		return false
	}
	primary.Path, primary.RawPath, primary.RawQuery = r.URL.Path, r.URL.RawPath, r.URL.RawQuery
	http.Redirect(w, r, primary.String(), http.StatusSeeOther)
	return true
}
