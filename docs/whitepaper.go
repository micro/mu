package docs

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"mu/internal/app"
	"mu/internal/data"
)

const pdfCacheKey = "whitepaper.pdf"

// cached PDF bytes (loaded from disk or generated once)
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

		// Hash the markdown to detect changes
		hash := sha256.Sum256(content)
		currentHash := hex.EncodeToString(hash[:8]) // short hash

		// Try loading cached PDF from disk
		cached, err := data.LoadFile(pdfCacheKey)
		if err == nil && len(cached) > 0 {
			// Check if the hash marker file matches
			hashBytes, _ := data.LoadFile(pdfCacheKey + ".hash")
			if string(hashBytes) == currentHash {
				pdfBytes = cached
				return
			}
		}

		// Generate fresh PDF
		pdfBytes = generateWhitepaperPDF(string(content))

		// Cache to disk
		dir := os.ExpandEnv("$HOME/.mu/data")
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, pdfCacheKey), pdfBytes, 0644)
		data.SaveFile(pdfCacheKey+".hash", currentHash)
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
