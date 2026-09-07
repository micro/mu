package news

import (
	htmlpkg "html"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/internal/imageproxy"
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
	if category == "" {
		latest := make([]*Post, 0, len(categories))
		covered := map[string]bool{}
		for _, p := range items {
			if !covered[p.Category] {
				latest = append(latest, p)
				covered[p.Category] = true
			}
		}
		items = latest
	}
	page, start, end := app.ReadingPage(r, len(items), 20)
	var b strings.Builder
	b.WriteString(`<form id="news-search" class="search-bar" action="/news" method="GET"><input id="news-query" name="query" type="search" placeholder="Search news" aria-label="Search news" maxlength="256"><button type="submit">Search</button></form>`)
	b.WriteString(app.ReadingFilters("/news", category, categories))
	if category == "" {
		b.WriteString(`<h2>Headlines</h2>`)
	}
	b.WriteString(`<div class="reading-list">`)
	if len(items) == 0 {
		b.WriteString(`<p>No articles in this category yet.</p>`)
	}
	for _, p := range items[start:end] {
		articleURL := "/news?id=" + url.QueryEscape(p.ID)
		b.WriteString(`<article id="reading-` + htmlpkg.EscapeString(p.ID) + `" class="reading-row news-reading-row">`)
		b.WriteString(`<a class="news-reading-image" href="` + articleURL + `" aria-label="` + htmlpkg.EscapeString(p.Title) + `">`)
		if p.Image != "" {
			b.WriteString(`<img src="` + htmlpkg.EscapeString(imageproxy.URL(p.Image)) + `" alt="" loading="lazy" onerror="this.hidden=true">`)
		}
		b.WriteString(`</a>`)

		b.WriteString(`<div class="news-reading-content">`)
		if p.Category != "" {
			b.WriteString(`<div class="category-header"><a class="news-topic" href="/news?category=` + url.QueryEscape(p.Category) + `">` + htmlpkg.EscapeString(p.Category) + `</a></div>`)
		}
		b.WriteString(`<h3><a href="` + articleURL + `">` + htmlpkg.EscapeString(p.Title) + `</a></h3><div class="reading-meta">` + htmlpkg.EscapeString(getDomain(p.URL)+" · "+app.TimeAgo(p.PostedAt)) + `</div>`)

		description := []rune(htmlToText(p.Description))
		if len(description) > 240 {
			description = append(description[:240], '…')
		}
		if len(description) > 0 {
			b.WriteString(`<p>` + htmlpkg.EscapeString(string(description)) + `</p>`)
		}
		b.WriteString(app.ReadingActions(r, p.ID) + `</div></article>`)
	}
	b.WriteString(app.ReadingPages("/news", category, page, len(items), 20))
	return b.String() + `</div>` + app.ReadingCSS + `<style>
.news-reading-row{display:flex;gap:20px;align-items:flex-start}
.news-reading-content{flex:1;min-width:0}
.news-reading-image{flex:0 0 200px;display:block;aspect-ratio:4/3;background:var(--hover-background,#eee);border-radius:var(--border-radius,6px)}
.news-reading-image img[hidden]{display:none}
.news-reading-image img{display:block;width:100%;aspect-ratio:4/3;object-fit:cover;border-radius:var(--border-radius,6px)}
.news-reading-row .category-header{margin-bottom:6px}
.news-topic{display:inline-block;padding:3px 8px;border:1px solid var(--card-border,#ddd);border-radius:var(--border-radius,6px);background:var(--card-background,#fff);color:var(--text-secondary,#555);font-size:12px;font-weight:600;text-decoration:none}
@media(max-width:600px){.news-reading-row{flex-direction:column;gap:12px}.news-reading-content{width:100%}.news-reading-image{flex-basis:auto;width:100%;aspect-ratio:16/9}.news-reading-image img{aspect-ratio:16/9}}
</style>`
}

func feedBody(r *http.Request, posts []*Post) string {
	if len(posts) == 0 && newsBodyHtml != "" {
		return newsBodyHtml
	}
	return browse(r, posts)
}
