package video

import (
	"html"
	"mu/internal/app"
	"mu/internal/data"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

func browse(r *http.Request, all map[string]Channel) string {
	category := r.URL.Query().Get("category")
	categories := []string{}
	seenCat := map[string]bool{}
	seen := map[string]bool{}
	items := []*Result{}
	for cat, ch := range all {
		if !seenCat[cat] {
			seenCat[cat] = true
			categories = append(categories, cat)
		}
		for _, v := range ch.Videos {
			if v == nil || seen[v.ID] || category != "" && category != cat {
				continue
			}
			seen[v.ID] = true
			items = append(items, v)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Published.Equal(items[j].Published) {
			return items[i].ID < items[j].ID
		}
		return items[i].Published.After(items[j].Published)
	})
	page, start, end := app.ReadingPage(r, len(items), 12)
	var b strings.Builder
	b.WriteString(app.ReadingFilters("/video", category, categories))
	b.WriteString(`<div class="video-grid">`)
	if len(items) == 0 {
		b.WriteString(`<p>No videos in this category yet.</p>`)
	}
	for _, v := range items[start:end] {
		b.WriteString(`<article id="reading-` + html.EscapeString("video_"+v.ID) + `" class="reading-row"><a href="/video?id=` + url.QueryEscape(v.ID) + `"><img src="` + html.EscapeString(thumbSrc(v.ID, v.Thumbnail)) + `" loading="lazy" alt=""><h3>` + html.EscapeString(v.Title) + `</h3></a><div class="reading-meta">` + html.EscapeString(v.Channel+" · "+app.TimeAgo(v.Published)) + `</div>` + app.ReadingActions(r, "video_"+v.ID) + `</article>`)
	}
	return b.String() + `</div>` + app.ReadingPages("/video", category, page, len(items), 12) + app.ReadingCSS + `<style>.video-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:24px}.video-grid .reading-row{padding:0 0 16px;min-width:0}.video-grid img{width:100%;aspect-ratio:16/9;object-fit:cover;border-radius:8px}.video-grid h3{font-size:16px}.video-grid a{text-decoration:none}@media(max-width:1000px){.video-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:600px){.video-grid{grid-template-columns:1fr}}</style>`
}

func watchTitle(id string) (string, string) {
	e := data.ByID("video_" + id)
	if e == nil || e.Owner != "" || e.Type != data.KindVideo {
		return "Video", ""
	}
	channel, _ := e.Metadata["channel"].(string)
	return e.Title, channel
}
