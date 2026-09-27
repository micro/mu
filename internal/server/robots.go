package server

import "net/http"

// Crawl guidance only; authentication still protects private resources.
func robotsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(`User-agent: *
Allow: /
Disallow: /account
Disallow: /admin
Disallow: /api/
Disallow: /mcp
Disallow: /oauth
Disallow: /login
Disallow: /signup
Disallow: /logout
Disallow: /token
Disallow: /home
Disallow: /agent
Disallow: /inbox
Disallow: /work
Disallow: /mail
Disallow: /chat
Disallow: /groups
Disallow: /events
Disallow: /tasks
Disallow: /notes
Disallow: /files
Disallow: /bookmarks
Disallow: /usage
Disallow: /billing
Disallow: /.well-known/jmap
Disallow: /*?session=
Disallow: /*?continue=
`))
}
