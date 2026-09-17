package islam

import (
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
)

// Handler is a reading/search surface over published sources, not an API form.
func Handler(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	collection := r.URL.Query().Get("collection")
	var b strings.Builder
	b.WriteString(`<p class="note">Quran, hadith and Islamic reference works from <a href="https://aslam.org">Aslam</a>.</p><form class="search-bar" method="GET" action="/islam"><input type="search" name="q" aria-label="Search Islamic sources" placeholder="Search a topic or passage" value="` + html.EscapeString(q) + `"><select name="collection" aria-label="Collection"><option value="">All sources</option>`)
	for _, c := range []struct{ key, label string }{{"quran", "Quran"}, {"hadith", "Hadith"}, {"adhkar", "Adhkar"}, {"names", "Names of Allah"}, {"seerah", "Seerah"}, {"salihin", "Riyad as-Salihin"}, {"ghazali", "Al-Ghazali"}, {"islamqa", "Questions and answers"}} {
		selected := ""
		if c.key == collection {
			selected = " selected"
		}
		b.WriteString(`<option value="` + c.key + `"` + selected + `>` + c.label + `</option>`)
	}
	b.WriteString(`</select><button>Search</button></form>`)
	if q != "" {
		var rsp SearchResponse
		if err := (Server{}).Search(r.Context(), &SearchRequest{Query: q, Collection: collection, Limit: 20}, &rsp); err != nil {
			b.WriteString(`<p class="notice">Could not load sources. <a href="https://aslam.org/search?q=` + url.QueryEscape(q) + `">Search on Aslam</a>.</p>`)
		} else {
			if len(rsp.Results) == 0 {
				b.WriteString(`<p class="note">No matching passages. Try a shorter phrase or another source.</p>`)
			}
			for _, item := range rsp.Results {
				b.WriteString(`<article class="record-card"><div class="metadata-row">` + app.Pill(item.Kind) + `<span>` + html.EscapeString(item.Source) + `</span></div><h3><a href="` + html.EscapeString(item.URL) + `">` + html.EscapeString(item.Title) + `</a></h3><p class="pre-line">` + html.EscapeString(item.Content) + `</p><div class="form-actions"><a class="mini-btn" href="` + html.EscapeString(item.URL) + `">Read passage</a></div></article>`)
			}
		}
	} else {
		b.WriteString(`<div class="collection-list">`)
		for _, c := range []struct{ name, path, description string }{{"Quran", "/quran", "Arabic text, translation and commentary"}, {"Hadith", "/hadith", "Read the source and its reference"}, {"Seerah", "/seerah", "The life of the Prophet"}, {"Adhkar", "/adhkar", "Daily remembrance and supplications"}} {
			b.WriteString(app.CollectionItem("https://aslam.org"+c.path, c.name, c.description, "Read on Aslam"))
		}
		b.WriteString(`</div>`)
	}
	app.Respond(w, r, app.Response{Title: "Islam", HTML: b.String()})
}
