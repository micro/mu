package app

import "html"

// SectionLink opens the full collection beneath a compact preview.
func SectionLink(label, href string) string {
	return `<a class="section-link" href="` + html.EscapeString(href) + `">` + html.EscapeString(label) + ` →</a>`
}
