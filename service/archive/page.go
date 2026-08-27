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
//
// Ten, not fifty. Nobody reads to the fiftieth hit of a search they can refine
// by typing another word, and every row carries its content — so the other
// forty were an article body each, read off disk, scored, rendered, and
// scrolled past. A search that answers in the first few is a search that
// worked; one that does not is a query to change, not a longer list.
const resultsShown = 10

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
	app.Respond(w, r, app.Response{Title: "Archive", Description: "Everything this instance has collected, searchable as one thing", HTML: b.String()})
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
		`<div class="ar-meta">` + app.Pill(e.Type) +
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
	// app.PillLink, not a fifth hand-drawn chip.
	//
	// This was .ar-chip: a border, a radius, a padding and a white-on-#111
	// selected state, which is app.Pill with different numbers. It had the bug
	// every hand-drawn copy had — a:visited outranked .ar-chip.on, so the
	// selected category was black on black once clicked, which for the selected
	// one is always. That is fixed at the source now (see the a:visited rule in
	// mu.css), and this stops being a fifth place for it to come back.
	chip := func(label, kind string, count int) string {
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
		text := label
		if count > 0 {
			text += " " + app.Count(count)
		}
		return app.PillLink(text, href, kind == active)
	}

	total := 0
	for _, k := range kinds {
		total += k.Count
	}

	var b strings.Builder
	b.WriteString(`<div class="app-filters">` + chip("Everything", "", total))
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
.ar-empty{font-size:14px;color:#888;line-height:1.6}
.ar-row{padding:12px 0;border-bottom:1px solid #f4f4f4}
/* A kind and a time are two facts, and this had nothing between them — the pill
   and the text were adjacent nodes in a block, so the line read "news2 hours
   ago". A gap on the row rather than a margin on .pill: that primitive is
   shared with the inbox and the agent, it already declares flex:none for
   exactly this, and every other place it is used sits in a flex row with a gap.
   This was the one that did not. */
.ar-meta{display:flex;align-items:center;gap:7px;font-size:11px;color:#bbb;margin-bottom:3px}
.ar-title{font-size:15px;color:#111;font-weight:500;text-decoration:none;display:block}
a.ar-title:hover{text-decoration:underline}
.ar-body{font-size:13px;color:#888;line-height:1.55;margin-top:3px}
@media (max-width:640px){
  .ar-form{flex-wrap:wrap}
  .ar-input{flex-basis:100%}
}
</style>`
