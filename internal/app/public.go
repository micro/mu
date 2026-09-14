package app

import (
	"html"
	"net/http"
)

// RespondPublic uses the landing shell for informational pages.
func RespondPublic(w http.ResponseWriter, r *http.Request, resp Response) {
	if WantsJSON(r) {
		Respond(w, r, resp)
		return
	}
	page := RenderIndex(Index{
		Title: html.EscapeString(resp.Title), Description: html.EscapeString(resp.Description),
		TopRight: `<a href="/">Micro</a>`,
		Body:     `<article class="public-page"><h1>` + html.EscapeString(resp.Title) + `</h1>` + resp.HTML + `</article>`,
		Footer:   FooterLinks(),
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, private")
	w.Write([]byte(page)) //nolint:errcheck
}
