// Package web embeds the focused client. It has no agent or account dependencies.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var assets embed.FS

// Handler serves the exact same static files distributed in mu-web.zip.
// Mount at /app/. The build includes every design asset; no runtime HTML is used.
func Handler() http.Handler {
	files, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	server := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self' https: http://localhost:* http://127.0.0.1:* http://[::1]:*; img-src 'self' data: https:; media-src 'self' https:; frame-src https://www.youtube-nocookie.com; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		server.ServeHTTP(w, r)
	})
}
