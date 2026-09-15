// Package web owns the browser client and its embedded production assets.
package web

import (
	"embed"
	"encoding/json"
	"html"
	"io/fs"
	"mu/internal/auth"
	"net/http"
	"strings"
)

//go:embed dist
var files embed.FS

// Page serves an HTML view; JSON and mutations remain with the owning handler.
func Page(w http.ResponseWriter, r *http.Request, title string, initial ...any) bool {
	if r.Header.Get("Upgrade") != "" || r.Method != http.MethodGet || strings.Contains(r.Header.Get("Accept"), "application/json") {
		return false
	}
	auth.SetCSRFCookie(w, r)
	page, err := files.ReadFile("dist/index.html")
	if err != nil {
		http.Error(w, "Web client unavailable", 503)
		return true
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Add("Vary", "Accept")
	var state any
	if len(initial) > 0 {
		state = initial[0]
	} else {
		_, acc := auth.TrySession(r)
		var identity any
		if acc != nil {
			identity = map[string]any{"id": acc.ID, "name": acc.Name, "admin": acc.Admin}
		}
		state = map[string]any{"account": identity, "csrf": auth.CSRFToken(r)}
	}
	bootstrap, err := json.Marshal(state)
	if err != nil {
		http.Error(w, "Web client unavailable", 500)
		return true
	}
	content := strings.Replace(string(page), "<title>Micro</title>", "<title>"+html.EscapeString(title)+" | Micro</title>", 1)
	if r.URL.Path != "/" {
		start, end := strings.Index(content, "<!--landing-->"), strings.Index(content, "<!--/landing-->")
		if start >= 0 && end > start {
			content = content[:start] + content[end+len("<!--/landing-->"):]
		}
	}
	content = strings.Replace(content, "</head>", `<script id="client-state" type="application/json">`+string(bootstrap)+`</script></head>`, 1)
	w.Write([]byte(content))
	return true
}

func Assets(w http.ResponseWriter, r *http.Request) {
	sub, _ := fs.Sub(files, "dist")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.StripPrefix("/client/assets/", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
}
