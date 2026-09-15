// Package apps owns the first-party application catalogue and browser routes.
// Service handlers continue to own data, authorisation and protocol endpoints.
package apps

import (
	_ "embed"
	"encoding/json"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/web"
	"net/http"
	"strings"
)

//go:embed catalog.json
var catalog []byte

type Entry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func Entries() []Entry { var entries []Entry; _ = json.Unmarshal(catalog, &entries); return entries }

// Page wraps only an application's HTML entry. It never intercepts data reads,
// writes, downloads, callbacks or protocol traffic.
func Page(next http.HandlerFunc, title string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mail" && (r.URL.Query().Get("action") == "download_attachment" || r.URL.Query().Get("action") == "view_raw") {
			next(w, r)
			return
		}
		if r.URL.Path == "/video" && (r.URL.Query().Get("id") != "" || r.URL.Query().Get("playlist") != "" || r.URL.Query().Get("channel") != "") {
			next(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/admin") {
			if _, _, err := auth.RequireAdmin(r); err != nil {
				app.Forbidden(w, r, "Admin access required")
				return
			}
		}
		if r.URL.Path == "/chat" && r.URL.Query().Get("with") != "" {
			next(w, r)
			return
		}
		if web.Page(w, r, title) {
			return
		}
		next(w, r)
	}
}
func Match(path string) (Entry, bool) {
	for _, a := range Entries() {
		if a.Path == strings.TrimSuffix(path, "/") {
			return a, true
		}
	}
	return Entry{}, false
}
func CatalogueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(catalog)
}
