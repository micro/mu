package archive

// The page, at /archive.
//
// What is on it is the fact that was hardest to establish: how much is in
// there, and of what. Six services have been writing to one index for a long
// time and the count was not on any screen — the admin stores table said the
// file was 366MB, which is a size and not an answer.
//
// It is a search box over that, and unlike /recall it shows something before
// you ask: the kinds and their counts. Recall is somebody's private
// correspondence and an unasked-for list of it would be a page that reads your
// mail at you; this is what the instance has collected about the world, and the
// shape of it is the interesting part.

import (
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/data"
)

// resultsShown bounds one page of results.
const resultsShown = 50

// Handler serves /archive. No session required: everything it can show is
// public by construction — an entry with an owner is never returned.
func Handler(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))

	var entries []*data.IndexEntry
	switch {
	case query != "":
		var opts []data.SearchOption
		if kind != "" {
			opts = append(opts, data.WithType(kind))
		}
		entries = data.Search(query, resultsShown, opts...)
	case kind != "":
		entries = data.ByType(kind, resultsShown)
	}

	kinds := data.Kinds()

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{
			"query": query, "kind": kind, "kinds": kinds, "results": entries})
		return
	}

	var b strings.Builder
	b.WriteString(`<div class="ar">`)
	b.WriteString(`<p class="lens-lead">Everything this instance has collected — the news it reads, ` +
		`the video it watches, the markets it follows, what it has written. One search across ` +
		`all of it. What you have said to an agent is somewhere else, in ` +
		app.TextLink("Recall", "/recall") + `.</p>`)

	b.WriteString(`<form method="GET" action="/archive" class="ar-form">`)
	if kind != "" {
		b.WriteString(`<input type="hidden" name="kind" value="` + html.EscapeString(kind) + `">`)
	}
	b.WriteString(`<input class="ar-input" type="search" name="q" placeholder="Search the archive" ` +
		`value="` + html.EscapeString(query) + `" autofocus>` +
		`<button class="ar-go" type="submit">Search</button></form>`)

	b.WriteString(kindChips(kinds, query, kind))

	switch {
	case query == "" && kind == "":
		b.WriteString(`<p class="ar-empty">Pick a kind, or search across all of them.</p>`)
	case len(entries) == 0 && query != "":
		b.WriteString(`<p class="ar-empty">Nothing in the archive mentions <strong>` +
			html.EscapeString(query) + `</strong>.</p>`)
	case len(entries) == 0:
		b.WriteString(`<p class="ar-empty">Nothing of that kind is archived.</p>`)
	default:
		for _, e := range entries {
			b.WriteString(row(e))
		}
	}

	b.WriteString(`</div>` + pageCSS)
	w.Write([]byte(app.RenderHTMLForRequest("Archive",
		"Everything this instance has collected, searchable as one thing", b.String(), r)))
}

// row is one entry: what it is, when it was collected, and enough of it to know
// whether it is the one you wanted.
func row(e *data.IndexEntry) string {
	title := strings.TrimSpace(e.Title)
	if title == "" {
		title = "Untitled"
	}
	// The link, where the entry knows where it came from. Metadata is whatever
	// the service that indexed it put there, so this asks rather than assumes.
	href := ""
	if e.Metadata != nil {
		if u, ok := e.Metadata["url"].(string); ok {
			href = u
		} else if u, ok := e.Metadata["link"].(string); ok {
			href = u
		}
	}

	head := `<span class="ar-title">` + html.EscapeString(trim(title, 140)) + `</span>`
	if href != "" {
		head = `<a class="ar-title" href="` + html.EscapeString(href) + `" rel="noopener">` +
			html.EscapeString(trim(title, 140)) + `</a>`
	}

	return `<div class="ar-row">` +
		`<div class="ar-meta"><span class="ar-kind">` + html.EscapeString(e.Type) + `</span>` +
		html.EscapeString(app.TimeAgo(e.IndexedAt)) + `</div>` + head +
		`<div class="ar-body">` + html.EscapeString(trim(e.Content, 260)) + `</div></div>`
}

// kindChips is what is in the archive and how much of it, doubling as the
// filter. The counts are the point — they are the answer to "what is in there",
// which nothing else in the product could give.
func kindChips(kinds []data.Kind, query, active string) string {
	if len(kinds) == 0 {
		return ""
	}
	chip := func(label, kind string, count int) string {
		cls := "ar-chip"
		if kind == active {
			cls += " on"
		}
		q := url.Values{}
		if query != "" {
			q.Set("q", query)
		}
		if kind != "" {
			q.Set("kind", kind)
		}
		href := "/archive"
		if len(q) > 0 {
			href += "?" + q.Encode()
		}
		out := `<a class="` + cls + `" href="` + href + `">` + html.EscapeString(label)
		if count > 0 {
			out += ` <span class="ar-n">` + app.Count(count) + `</span>`
		}
		return out + `</a>`
	}

	total := 0
	for _, k := range kinds {
		total += k.Count
	}

	var b strings.Builder
	b.WriteString(`<div class="ar-chips">` + chip("Everything", "", total))
	for _, k := range kinds {
		b.WriteString(chip(k.Name, k.Name, k.Count))
	}
	b.WriteString(`</div>`)
	return b.String()
}

const pageCSS = `<style>
.ar{max-width:760px}
.ar-form{display:flex;gap:8px;margin:0 0 14px}
.ar-input{flex:1;font:inherit;font-size:15px;padding:9px 13px;border:1px solid #e2e2e2;border-radius:8px}
.ar-input:focus{outline:none;border-color:#bbb}
.ar-go{font:inherit;font-size:14px;padding:8px 18px;border:1px solid #111;background:#111;color:#fff;border-radius:8px;cursor:pointer}
.ar-chips{display:flex;flex-wrap:wrap;gap:6px;margin:0 0 18px}
.ar-chip{border:1px solid #eee;border-radius:999px;padding:4px 12px;font-size:12px;color:#666;text-decoration:none;white-space:nowrap}
.ar-chip:hover{border-color:#ddd;color:#111}
.ar-chip.on{background:#111;border-color:#111;color:#fff}
.ar-n{opacity:.6}
.ar-empty{font-size:14px;color:#888;line-height:1.6}
.ar-row{padding:12px 0;border-bottom:1px solid #f4f4f4}
.ar-meta{font-size:11px;color:#bbb;margin-bottom:3px}
.ar-kind{border:1px solid #eee;border-radius:999px;padding:1px 7px;margin-right:7px;color:#777}
.ar-title{font-size:15px;color:#111;font-weight:500;text-decoration:none;display:block}
a.ar-title:hover{text-decoration:underline}
.ar-body{font-size:13px;color:#888;line-height:1.55;margin-top:3px}
@media (max-width:640px){
  .ar-form{flex-wrap:wrap}
  .ar-input{flex-basis:100%}
}
</style>`
