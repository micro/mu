package app

import (
	"fmt"
	"html"
	"mu/internal/auth"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// SaveControl is rendered per request, so neither saved state nor CSRF tokens
// enter the shared feed HTML cache.
func SaveControl(r *http.Request, ref string) string {
	if r.URL.Query().Get("saved") == ref {
		return `<a href="/saved">Saved ✓</a>`
	}
	if _, acc := auth.TrySession(r); acc == nil {
		return `<a href="/saved?item=` + url.QueryEscape(ref) + `">Save</a>`
	}
	return `<form method="POST" action="/saved" class="reading-save"><input type="hidden" name="action" value="add"><input type="hidden" name="ref" value="` + html.EscapeString(ref) + `"><input type="hidden" name="back" value="` + html.EscapeString(r.URL.RequestURI()) + `"><input type="hidden" name="csrf_token" value="` + html.EscapeString(auth.CSRFToken(r)) + `"><button type="submit">Save</button></form>`
}
func ReadingActions(r *http.Request, ref string) string {
	return `<div class="reading-actions">` + SaveControl(r, ref) + `<a href="/agent/micro?item=` + url.QueryEscape(ref) + `">Ask Micro</a></div>`
}
func ReadingFilters(path, active string, categories []string) string {
	sort.Strings(categories)
	b := `<div class="app-filters">` + PillLink("All", path, active == "")
	for _, c := range categories {
		b += PillLink(c, path+"?category="+url.QueryEscape(c), c == active)
	}
	return b + `</div>`
}
func ReadingPage(r *http.Request, total, size int) (int, int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	last := max(1, (total+size-1)/size)
	page = max(1, min(page, last))
	start := min((page-1)*size, total)
	return page, start, min(start+size, total)
}
func ReadingPages(path, category string, page, total, size int) string {
	last := max(1, (total+size-1)/size)
	if last == 1 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<nav class="reading-actions" aria-label="Pages">`)
	link := func(label string, p int) {
		q := url.Values{"page": {strconv.Itoa(p)}}
		if category != "" {
			q.Set("category", category)
		}
		fmt.Fprintf(&b, `<a href="%s?%s">%s</a>`, path, html.EscapeString(q.Encode()), label)
	}
	if page > 1 {
		link("← Previous", page-1)
	}
	fmt.Fprintf(&b, `<span>Page %d of %d</span>`, page, last)
	if page < last {
		link("Next →", page+1)
	}
	return b.String() + `</nav>`
}

const ReadingCSS = `<style>
.reading-row{padding:18px 0;border-bottom:1px solid var(--border,#eee)}.reading-row h3{font-size:18px;line-height:1.4;margin:5px 0}.reading-row p{font-size:14px;line-height:1.6;color:var(--text-muted,#666);margin:6px 0}.reading-meta{font-size:12px;color:var(--text-muted,#777)}.reading-actions{display:flex;align-items:center;gap:16px;flex-wrap:wrap;margin:12px 0;font-size:14px}.reading-actions form,.reading-save{display:inline;margin:0}.reading-save button{font:inherit;background:none;border:0;padding:0;color:inherit;cursor:pointer;text-decoration:underline}.reading-list{max-width:840px}.reading-row h3 a{text-decoration:none}.reading-row h3 a:hover{text-decoration:underline}
</style>`
