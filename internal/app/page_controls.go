package app

import (
	"html"
	"strings"
)

// ViewLink is a destination within a page or a filter over its contents.
type ViewLink struct{ Key, Label, URL string }

// ViewNavigation renders both levels of page navigation through one component.
// Filters are compact; page tabs have an underline identifying the current view.
func ViewNavigation(label, active string, links []ViewLink, filter bool) string {
	class := "view-tabs"
	if filter {
		class = "view-filters"
	}
	var b strings.Builder
	b.WriteString(`<nav class="` + class + `" aria-label="` + html.EscapeString(label) + `">`)
	for _, link := range links {
		current := ""
		if link.Key == active {
			current = ` aria-current="page"`
		}
		b.WriteString(`<a href="` + html.EscapeString(link.URL) + `"` + current + `>` + html.EscapeString(link.Label) + `</a>`)
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// PageControls keeps a stable page introduction above tabs and view controls,
// with one boundary before the results or editor.
func PageControls(description, tabs, controls string) string {
	body := `<div class="page-controls"><p class="text-muted">` + html.EscapeString(description) + `</p>` + tabs
	if controls != "" {
		body += `<div class="view-controls">` + controls + `</div>`
	}
	return body + `</div>`
}
