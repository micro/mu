package app

import (
	"html"
	"strings"
)

// UI layout helpers for consistent rendering.
// Use these wrappers + mu.css classes.

// SearchBar renders a search input with search button
func SearchBar(action, placeholder, query string) string {
	var b strings.Builder
	b.WriteString(`<form class="search-bar" action="`)
	b.WriteString(action)
	b.WriteString(`" method="GET"><input type="text" name="q" placeholder="`)
	b.WriteString(placeholder)
	b.WriteString(`" value="`)
	b.WriteString(html.EscapeString(query))
	b.WriteString(`"><button type="submit">Search</button></form>`)
	return b.String()
}

// ActionLink renders a primary action link (e.g., "+ New Note")
func ActionLink(href, label string) string {
	return `<a href="` + href + `" class="btn">` + html.EscapeString(label) + `</a>`
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

// CardDiv wraps content in a card container
func CardDiv(content string) string {
	return `<div class="card">` + content + `</div>`
}

// CardDivClass wraps content in a card with additional classes
func CardDivClass(class, content string) string {
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
