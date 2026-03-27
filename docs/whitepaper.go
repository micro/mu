package docs

import (
	"net/http"
	"strings"
	"sync"

	"mu/internal/app"
)

// cached PDF bytes (generated once on first request)
var (
	pdfOnce  sync.Once
	pdfBytes []byte
)

func getWhitepaperMarkdown() ([]byte, error) {
	return docsFS.ReadFile("WHITEPAPER.md")
}

func getWhitepaperPDF() []byte {
	pdfOnce.Do(func() {
		content, err := getWhitepaperMarkdown()
		if err != nil {
			return
		}
		pdfBytes = generateWhitepaperPDF(string(content))
	})
	return pdfBytes
}

// WhitepaperHandler serves the whitepaper at /whitepaper and /whitepaper.pdf
func WhitepaperHandler(w http.ResponseWriter, r *http.Request) {
	// Serve PDF at /whitepaper.pdf
	if strings.HasSuffix(r.URL.Path, ".pdf") {
		pdf := getWhitepaperPDF()
		if pdf == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", "inline; filename=\"mu-whitepaper.pdf\"")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(pdf)
		return
	}

	// Serve HTML at /whitepaper
	content, err := getWhitepaperMarkdown()
	if err != nil {
		http.NotFound(w, r)
		return
	}

	rendered := app.Render(content)

	html := `<div class="docs"><div class="docs-content">` + string(rendered) +
		`<p style="margin-top:2em"><a href="/whitepaper.pdf">Download PDF</a></p></div></div>`

	page := app.RenderHTMLForRequest("Whitepaper", "Mu: A Unified Service Network with Native Payments for Humans and Autonomous Agents", html, r)
	w.Write([]byte(page))
}
