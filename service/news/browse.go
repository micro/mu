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
	b.WriteString(app.RecentSearches("news-search", "mu-news-recent"))
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
		b.WriteString(articleCover(articleURL, p.URL, p.Image, p.Title))

		b.WriteString(`<div class="news-reading-content">`)
		b.WriteString(`<h3><a href="` + articleURL + `">` + htmlpkg.EscapeString(p.Title) + `</a></h3><div class="reading-meta">`)
		if p.Category != "" {
			b.WriteString(`<a href="/news?category=` + url.QueryEscape(p.Category) + `">` + htmlpkg.EscapeString(p.Category) + `</a>`)
		}
		b.WriteString(`<span>` + htmlpkg.EscapeString(getDomain(p.URL)+" · "+app.TimeAgo(p.PostedAt)) + `</span></div>`)

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
	return b.String() + `</div>` + ``
}

func feedBody(r *http.Request, posts []*Post) string {
	if len(posts) == 0 && newsBodyHtml != "" {
		return newsBodyHtml
	}
	return browse(r, posts)
}

// articleCover preserves the media column even without a publisher image.
func articleCover(href, source, image, title string) string {
	domain := getDomain(source)
	if domain == "" {
		domain = "News"
	}
	cover := `<a class="news-reading-image media-cover" href="` + htmlpkg.EscapeString(href) + `" aria-label="` + htmlpkg.EscapeString(title) + `"><span class="media-cover-label" aria-hidden="true">` + htmlpkg.EscapeString(domain) + `</span>`
	if image != "" {
		cover += `<img src="` + htmlpkg.EscapeString(imageproxy.URL(image)) + `" alt="" loading="lazy" data-cover-image>`
	}
	return cover + `</a>`
}
