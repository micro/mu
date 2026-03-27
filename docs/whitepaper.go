package docs

import (
	"net/http"

	"mu/internal/app"
)

// WhitepaperHandler serves the whitepaper at /whitepaper and /whitepaper.pdf
func WhitepaperHandler(w http.ResponseWriter, r *http.Request) {
	content, err := docsFS.ReadFile("WHITEPAPER.md")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	rendered := app.Render(content)

	html := `<div class="docs"><div class="docs-content">` + string(rendered) + `</div></div>`

	page := app.RenderHTMLForRequest("Whitepaper", "Mu: A Network for Apps Without Ads, Algorithms, or Tracking", html, r)
	w.Write([]byte(page))
}
