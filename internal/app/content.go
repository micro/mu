package app

import (
	"fmt"
	"html"
	"strings"
)

// ContentItem represents any user-generated content for listing.
// Fill in the fields and call RenderItems for consistent rendering with admin controls.
type ContentItem struct {
	ID          string
	Title       string
	Href        string // link to detail page
	Description string // short preview text
	Meta        string // e.g. "Task · 500 credits · 2 Jan 2006"
	AuthorID    string
	Author      string
	DeleteURL   string // POST endpoint for deletion (empty = no delete)
}

// RenderItems renders a list of content items as cards with consistent layout.
// Admin and author get a delete link automatically.
func RenderItems(items []ContentItem, userID string, isAdmin bool) string {
	if len(items) == 0 {
		return `<div class="card"><p class="text-muted">Nothing here yet.</p></div>`
	}

	var sb strings.Builder
	for _, item := range items {
		sb.WriteString(`<div class="card">`)

		// Title
		if item.Href != "" {
			sb.WriteString(fmt.Sprintf(`<h4><a href="%s">%s</a></h4>`, item.Href, html.EscapeString(item.Title)))
		} else {
			sb.WriteString(fmt.Sprintf(`<h4>%s</h4>`, html.EscapeString(item.Title)))
		}

		// Meta line
		if item.Meta != "" {
			sb.WriteString(fmt.Sprintf(`<p class="text-sm text-muted">%s</p>`, item.Meta))
		}

		// Description
		if item.Description != "" {
			sb.WriteString(fmt.Sprintf(`<p class="text-sm">%s</p>`, html.EscapeString(item.Description)))
		}

		// Delete (admin or author)
		if item.DeleteURL != "" && (isAdmin || userID == item.AuthorID) {
			sb.WriteString(fmt.Sprintf(`<form method="POST" action="%s" onsubmit="return confirm('Delete this?')" style="margin-top:8px">`, item.DeleteURL))
			sb.WriteString(`<button type="submit" class="text-sm text-muted" style="background:none;border:none;padding:0;cursor:pointer;text-decoration:underline">delete</button>`)
			sb.WriteString(`</form>`)
		}

		sb.WriteString(`</div>`)
	}
	return sb.String()
}
