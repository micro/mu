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

// CSRFField is the hidden input that lets a form POST.
//
// Empty for a signed-out reader, who has no session to derive a token from and
// is not checked — see auth.ValidCSRF. Written once here because a search form
// that posts needs it and there are a dozen of those; each one spelling out the
// field name is a dozen places to get `_csrf` wrong, and getting it wrong fails
// open today and closed the day the grace period ends.
func CSRFField(token string) string {
	if token == "" {
		return ""
	}
	return `<input type="hidden" name="_csrf" value="` + html.EscapeString(token) + `">`
}

// SearchBar renders a search input with search button.
//
// POST, because what somebody types into it is theirs. A GET puts the words in
// the URL, and a URL comes to rest in the browser's history and in the access
// log of whatever terminates TLS in front of us — which for a self-hosted
// instance is an nginx or a Caddy logging the full URI by default. See AGENTS.md,
// "What may travel in a URL".
//
// The cost is that a result page cannot be linked to, and for a search over
// somebody's own mail that is the feature rather than the price: the back button
// goes to the unsearched page, which is where somebody who has finished looking
// wants to be. A search over public content is a different question and keeps
// its GET — see the allowlist in TestNothingPrivateTravelsInAURL.
func SearchBar(action, placeholder, query, csrf string) string {
	var b strings.Builder
	b.WriteString(`<form class="search-bar" action="`)
	b.WriteString(action)
	b.WriteString(`" method="POST">`)
	b.WriteString(CSRFField(csrf))
	b.WriteString(`<input type="text" name="q" placeholder="`)
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
	CSRF    string // Token for the search form, which posts — see SearchBar
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
		b.WriteString(SearchBar(opts.Search, "Search...", opts.Query, opts.CSRF))
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

// The instance's own agent was named here too, as a pair of display constants
// from before it had an account. It has one now and the name went with it —
// see auth.MicroID, which carries the rest of this note.

// --- Shared content components ---
// Used across blog, news, mail, and other packages
// for consistent rendering of common UI patterns.

// Column opens the page's text column.
//
// One width, everywhere. Pages picked their own — 600, 640, 680, 700, 720, 760,
// 820 — so the text changed width as you walked between them, and /privacy also
// centred itself, which moved it sideways as well. None of that was decided;
// each page picked a number the day it was written.
//
// 720 because it is what the two widest text pages already used and it is about
// 90 characters at the body size, which is the top of the readable range.
//
// A function rather than a class, because the pages that need it are building
// strings and a helper is one call rather than a div somebody has to remember
// to close in the right place — Close is its pair.
func Column() string { return `<div class="page-col">` }

// Close closes what Column opened.
func Close() string { return `</div>` }
