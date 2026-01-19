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

// PageOpts defines the standard page layout options
type PageOpts struct {
	Action  string // Primary action URL (shows button if set)
	Label   string // Action button label (default: "+ New")
	Search  string // Search endpoint (shows search bar if set)
	Query   string // Current search query
	Filters string // Filter HTML (tags, toggles) - rendered as-is
	Content string // Main content (grid, list, cards)
	Empty   string // Empty state message (shown if Content is empty)
}

// Page renders a standard page layout
// Structure: [Search Bar] [Action Button] [Filters] [Content or Empty]
func Page(opts PageOpts) string {
	var b strings.Builder

	// Search bar (at top)
	if opts.Search != "" {
		b.WriteString(SearchBar(opts.Search, "Search...", opts.Query))
	}

	// Action button (below search)
	if opts.Action != "" {
		label := opts.Label
		if label == "" {
			label = "+ New"
		}
		b.WriteString(`<div class="page-action">`)
		b.WriteString(ActionLink(opts.Action, label))
		b.WriteString(`</div>`)
	}

	// Filters (tags, toggles, etc.)
	if opts.Filters != "" {
		b.WriteString(`<div class="page-filters">`)
		b.WriteString(opts.Filters)
		b.WriteString(`</div>`)
	}

	// Content or empty state
	if opts.Content != "" {
		b.WriteString(opts.Content)
	} else if opts.Empty != "" {
		b.WriteString(Empty(opts.Empty))
	}

	return b.String()
}
