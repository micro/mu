// Package web owns the browser client and its embedded production assets.
package web

import (
	"embed"
	"html"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed dist
var files embed.FS

// Page serves an HTML view; JSON and mutations remain with the owning handler.
func Page(w http.ResponseWriter, r *http.Request, title string) bool {
	if r.Method != http.MethodGet || strings.Contains(r.Header.Get("Accept"), "application/json") {
		return false
	}
	page, err := files.ReadFile("dist/index.html")
	if err != nil {
		http.Error(w, "Web client unavailable", 503)
		return true
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Add("Vary", "Accept")
	w.Write([]byte(strings.Replace(string(page), "<title>Micro</title>", "<title>"+html.EscapeString(title)+" | Micro</title>", 1)))
	return true
}

func Assets(w http.ResponseWriter, r *http.Request) {
	sub, _ := fs.Sub(files, "dist")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.StripPrefix("/client/assets/", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
}
