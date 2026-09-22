package app

import (
	"html"
	"mu/internal/auth"
	"net/http"
)

// RespondPublic uses the landing shell for informational pages.
func RespondPublic(w http.ResponseWriter, r *http.Request, resp Response) {
	if WantsJSON(r) {
		Respond(w, r, resp)
		return
	}
	_, acc := auth.TrySession(r)
	page := ConsoleHTML(resp.Title, `<h1>`+html.EscapeString(resp.Title)+`</h1>`+resp.HTML, acc, r.URL.RequestURI())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, private")
	w.Write([]byte(page)) //nolint:errcheck
}
