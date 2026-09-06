package news

import (
	htmlpkg "html"
	"mu/internal/app"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

func browse(r *http.Request, posts []*Post) string {
	category := r.URL.Query().Get("category")
	categories := []string{}
	seen := map[string]bool{}
	items := []*Post{}
	for _, p := range dedupePosts(posts) {
		if p == nil {
			continue
		}
		if !seen[p.Category] {
			seen[p.Category] = true
			categories = append(categories, p.Category)
		}
		if category == "" || category == p.Category {
			items = append(items, p)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].PostedAt.After(items[j].PostedAt) })
	page, start, end := app.ReadingPage(r, len(items), 20)
	var b strings.Builder
	b.WriteString(app.ReadingFilters("/news", category, categories))
	b.WriteString(`<div class="reading-list">`)
	if len(items) == 0 {
		b.WriteString(`<p>No articles in this category yet.</p>`)
	}
	for _, p := range items[start:end] {
		b.WriteString(`<article id="reading-` + htmlpkg.EscapeString(p.ID) + `" class="reading-row"><div class="reading-meta">` + htmlpkg.EscapeString(getDomain(p.URL)+" · "+p.Category+" · "+app.TimeAgo(p.PostedAt)) + `</div><h3><a href="/news?id=` + url.QueryEscape(p.ID) + `">` + htmlpkg.EscapeString(p.Title) + `</a></h3>`)
		description := []rune(htmlToText(p.Description))
		if len(description) > 240 {
			description = append(description[:240], '…')
		}
		if len(description) > 0 {
			b.WriteString(`<p>` + htmlpkg.EscapeString(string(description)) + `</p>`)
		}
		b.WriteString(app.ReadingActions(r, p.ID) + `</article>`)
	}
	b.WriteString(app.ReadingPages("/news", category, page, len(items), 20))
	return b.String() + `</div>` + app.ReadingCSS
}
