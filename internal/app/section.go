package app

import "html"

// SectionLink opens the full collection beneath a compact preview.
func SectionLink(label, href string) string {
	return `<a class="section-link" href="` + html.EscapeString(href) + `">` + html.EscapeString(label) + ` →</a>`
}

// PreviewCard uses the same heading and container as Feed cards.
func PreviewCard(id, title, href, body string) string {
	heading := `<a class="card-head-link" href="` + html.EscapeString(href) + `">` + html.EscapeString(title) + `</a>`
	return Card(id, heading, `<div class="preview-rows">`+body+`</div>`)
}
