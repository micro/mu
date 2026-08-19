package app

import (
	"html"
	"strings"
)

// The components a page is built from.
//
// This file used to hold twenty-four of these and seventeen had no callers.
// That is worth stating plainly, because it is the same shape as everything
// else that went wrong with the UI: the abstraction was built, it was
// reasonable, and nothing adopted it — so every page hand-rolled its markup
// beside a library that already did the job.
//
// The dead ones are gone. What is left is what pages actually call, and the
// rule for adding one is that it has a second caller waiting. A component with
// no users is not a component, it is a proposal.
//
// Everything here escapes its own input and carries exactly one class, defined
// in internal/app/html/mu.css. See primitives.go for the smallest shapes.

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

// List wraps content in a card-list container
func List(content string) string {
	return `<div class="card-list">` + content + `</div>`
}

// Empty renders an empty state message
func Empty(message string) string {
	return `<p class="empty">` + html.EscapeString(message) + `</p>`
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

// --- System user ---
// The internal/system account used for automated posts, seeded threads, and AI responses.
// "micro" is already registered as a user account.
const (
	SystemUserID   = "micro"
	SystemUserName = "Micro"
)

// --- Shared content components ---
// Used across blog, news, mail, and other packages
// for consistent rendering of common UI patterns.
