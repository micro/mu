package app

import (
	"fmt"
	"html"
	"strings"
	"time"
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

// --- System user ---
// The internal/system account used for automated posts, seeded threads, and AI responses.
// "micro" is already registered as a user account.
const (
	SystemUserID   = "micro"
	SystemUserName = "Micro"
)

// --- Shared content components ---
// Used across social, blog, news, mail, and other packages
// for consistent rendering of common UI patterns.

// Category renders a topic/tag badge.
// If href is set, renders as a link; otherwise a span.
func Category(label, href string) string {
	escaped := html.EscapeString(label)
	if href != "" {
		return `<a href="` + href + `" class="category">` + escaped + `</a>`
	}
	return `<span class="category">` + escaped + `</span>`
}

// AuthorLink renders an author name as a profile link.
func AuthorLink(authorID, authorName string) string {
	return fmt.Sprintf(`<a href="/@%s" class="text-muted">%s</a>`,
		authorID, html.EscapeString(authorName))
}

// ItemMeta renders a metadata line with parts separated by " · ".
// Pass any combination of strings (category, author link, time, action links).
// Empty strings are skipped.
func ItemMeta(parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	if len(nonEmpty) == 0 {
		return ""
	}
	return `<div class="card-meta">` + strings.Join(nonEmpty, " · ") + `</div>`
}

// DeleteButton renders a delete link with a confirmation dialog.
// It creates a hidden form with _method=DELETE and submits on confirm.
func DeleteButton(action, label, confirmMsg string) string {
	if label == "" {
		label = "Delete"
	}
	if confirmMsg == "" {
		confirmMsg = "Are you sure?"
	}
	return fmt.Sprintf(`<a href="#" onclick="if(confirm('%s')){var f=document.createElement('form');f.method='POST';f.action='%s';var i=document.createElement('input');i.type='hidden';i.name='_method';i.value='DELETE';f.appendChild(i);document.body.appendChild(f);f.submit();}return false;" class="text-error">%s</a>`,
		html.EscapeString(confirmMsg),
		html.EscapeString(action),
		html.EscapeString(label))
}

// ReplyForm renders a reply/comment form with an optional hidden parent ID.
// Set parentName and parentValue for threaded replies (e.g. "parent_id", "123").
func ReplyForm(action, placeholder, parentName, parentValue string) string {
	if placeholder == "" {
		placeholder = "Write a reply..."
	}
	var b strings.Builder
	b.WriteString(`<form method="POST" action="`)
	b.WriteString(html.EscapeString(action))
	b.WriteString(`" class="blog-form card mt-5">`)
	if parentName != "" && parentValue != "" {
		b.WriteString(`<input type="hidden" name="`)
		b.WriteString(html.EscapeString(parentName))
		b.WriteString(`" value="`)
		b.WriteString(html.EscapeString(parentValue))
		b.WriteString(`">`)
	}
	b.WriteString(`<textarea name="content" rows="3" placeholder="`)
	b.WriteString(html.EscapeString(placeholder))
	b.WriteString(`" required></textarea>`)
	b.WriteString(`<button type="submit">Reply</button>`)
	b.WriteString(`</form>`)
	return b.String()
}

// InlineReplyForm renders a small reply form that starts hidden.
// Toggle visibility by ID: "rf-{id}".
func InlineReplyForm(id, action, parentName, parentValue string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<form id="rf-%s" method="POST" action="%s" style="display:none;margin-top:8px;">`,
		html.EscapeString(id), html.EscapeString(action)))
	if parentName != "" && parentValue != "" {
		b.WriteString(fmt.Sprintf(`<input type="hidden" name="%s" value="%s">`,
			html.EscapeString(parentName), html.EscapeString(parentValue)))
	}
	b.WriteString(`<textarea name="content" rows="2" placeholder="Reply..." required style="width:100%;font-size:13px;"></textarea>`)
	b.WriteString(`<button type="submit" style="margin-top:4px;font-size:12px;">Reply</button>`)
	b.WriteString(`</form>`)
	return b.String()
}

// ReplyLink renders a "Reply" link that toggles an InlineReplyForm visible.
func ReplyLink(formID string) string {
	return fmt.Sprintf(`<a href="#" class="text-muted" onclick="document.getElementById('rf-%s').style.display='block';this.style.display='none';return false;">Reply</a>`,
		html.EscapeString(formID))
}

// Section renders a section header.
func Section(title string) string {
	return `<h3 style="margin-top:20px;">` + html.EscapeString(title) + `</h3>`
}

// LoginPrompt renders a "Login to X" message with redirect.
func LoginPrompt(action, redirectTo string) string {
	return fmt.Sprintf(`<p class="text-muted mt-5"><a href="/login?redirect=%s">Login</a> to %s</p>`,
		html.EscapeString(redirectTo), html.EscapeString(action))
}

// Timestamp renders a time as "X ago" text.
func Timestamp(t time.Time) string {
	return TimeAgo(t)
}

// BackLink renders a "← Back to X" navigation link.
func BackLink(label, href string) string {
	return fmt.Sprintf(`<p class="mt-5"><a href="%s">← %s</a></p>`,
		html.EscapeString(href), html.EscapeString(label))
}
