package app

import "html"

// SectionLink opens the full collection beneath a compact preview.
func SectionLink(label, href string) string {
	return `<a class="section-link" href="` + html.EscapeString(href) + `">` + html.EscapeString(label) + ` →</a>`
}

// PreviewCard uses the same heading and container as Feed cards.
func PreviewCard(id, title, href, body string) string {
	heading := `<a class="card-head-link" href="` + html.EscapeString(href) + `">` + html.EscapeString(title) + `</a>`
	return SectionCard(id, heading, href, `<div class="preview-rows">`+body+`</div>`)
}

// SectionCard groups a trusted heading, navigation and body inside one bordered section.
// Heading and body are pre-rendered HTML; id and href are escaped here.
func SectionCard(id, heading, href, body string) string {
	more := ""
	if href != "" {
		more = `<a class="section-more" href="` + html.EscapeString(href) + `">More →</a>`
	}
	return `<section id="` + html.EscapeString(id) + `" class="section-card"><div class="section-card-head"><h4>` + heading + `</h4>` + more + `</div><div class="card"><div class="card-body">` + body + `</div></div></section>`
}
