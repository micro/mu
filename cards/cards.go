// Package cards provides layout helpers for consistent UI.
// Use these wrappers + mu.css classes. Keep render logic in each package.
package cards

import (
	"html"
	"strings"
)

// SearchHeader renders the standard search bar with optional new button
func SearchHeader(action, placeholder, query, newURL, newLabel string) string {
	var b strings.Builder
	b.WriteString(`<div class="search-bar">`)
	b.WriteString(`<form action="`)
	b.WriteString(action)
	b.WriteString(`" method="GET"><input type="text" name="q" placeholder="`)
	b.WriteString(placeholder)
	b.WriteString(`" value="`)
	b.WriteString(html.EscapeString(query))
	b.WriteString(`"></form>`)
	if newURL != "" {
		b.WriteString(`<a href="`)
		b.WriteString(newURL)
		b.WriteString(`" class="btn">`)
		if newLabel != "" {
			b.WriteString(newLabel)
		} else {
			b.WriteString(`+ New`)
		}
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// Grid wraps content in a card-grid container
func Grid(content string) string {
	return `<div class="card-grid">` + content + `</div>`
}

// List wraps content in a card-list container
func List(content string) string {
	return `<div class="card-list">` + content + `</div>`
}

// Row wraps content in a card-row container
func Row(content string) string {
	return `<div class="card-row">` + content + `</div>`
}

// Empty renders an empty state message
func Empty(message string) string {
	return `<p class="empty">` + html.EscapeString(message) + `</p>`
}

// Card wraps content in a card container
func Card(content string) string {
	return `<div class="card">` + content + `</div>`
}

// CardWithClass wraps content in a card with additional classes
func CardWithClass(class, content string) string {
	return `<div class="card ` + class + `">` + content + `</div>`
}

// Tags renders a list of tags
func Tags(tags []string, baseURL string) string {
	if len(tags) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="card-tags">`)
	for _, tag := range tags {
		if baseURL != "" {
			b.WriteString(`<a href="`)
			b.WriteString(baseURL)
			b.WriteString(html.EscapeString(tag))
			b.WriteString(`" class="tag">`)
			b.WriteString(html.EscapeString(tag))
			b.WriteString(`</a>`)
		} else {
			b.WriteString(`<span class="tag">`)
			b.WriteString(html.EscapeString(tag))
			b.WriteString(`</span>`)
		}
	}
	b.WriteString(`</div>`)
	return b.String()
}

// Title renders a card title with link
func Title(text, href string) string {
	if href != "" {
		return `<a href="` + href + `" class="card-title">` + html.EscapeString(text) + `</a>`
	}
	return `<span class="card-title">` + html.EscapeString(text) + `</span>`
}

// Meta renders metadata text
func Meta(content string) string {
	return `<div class="card-meta">` + content + `</div>`
}

// Desc renders description text
func Desc(text string) string {
	return `<p class="card-desc">` + html.EscapeString(text) + `</p>`
}
