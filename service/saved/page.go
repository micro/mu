package saved

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	store "mu/internal/saved"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	if r.Method == http.MethodPost && !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Invalid CSRF token")
		return
	}
	if err = r.ParseForm(); err != nil {
		app.BadRequest(w, r, "Could not read the form")
		return
	}
	query := ""
	kind := r.URL.Query().Get("kind")
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if r.Method == http.MethodPost {
		query = r.PostFormValue("query")
		kind = r.PostFormValue("kind")
		offset, _ = strconv.Atoi(r.PostFormValue("offset"))
		if r.URL.Path != "/saved/search" {
			if err = auth.CheckPostRate(sess.Account); err != nil {
				app.Error(w, r, http.StatusTooManyRequests, err.Error())
				return
			}
			var item *store.Item
			switch r.PostFormValue("action") {
			case "add":
				item, err = store.Add(sess.Account, store.Item{Ref: r.PostFormValue("ref"), URL: r.PostFormValue("url"), Title: r.PostFormValue("title"), Note: r.PostFormValue("note")})
			case "note":
				err = store.Annotate(sess.Account, r.PostFormValue("id"), r.PostFormValue("note"))
			case "delete":
				err = store.Remove(sess.Account, r.PostFormValue("id"))
			default:
				app.BadRequest(w, r, "Unknown action")
				return
			}
			if err != nil {
				app.BadRequest(w, r, err.Error())
				return
			}
			back := "/saved"
			if r.PostFormValue("action") == "note" {
				back += "?id=" + url.QueryEscape(r.PostFormValue("id"))
			}
			if item != nil {
				back += "?id=" + url.QueryEscape(item.ID)
			}
			// Only known public reading routes are accepted as a return destination.
			if u, e := url.Parse(r.PostFormValue("back")); e == nil && u.Host == "" && u.Scheme == "" && (u.Path == "/news" || u.Path == "/video" || u.Path == "/blog/post") {
				q := url.Values{}
				for _, name := range []string{"id", "category", "page"} {
					if value := u.Query().Get(name); len(value) <= 256 && value != "" {
						q.Set(name, value)
					}
				}
				if item != nil {
					q.Set("saved", item.Ref)
				}
				back = u.Path + "?" + q.Encode()
				if item != nil && q.Get("id") == "" {
					back += "#reading-" + url.QueryEscape(item.Ref)
				}
			}
			http.Redirect(w, r, back, http.StatusSeeOther)
			return
		}
	}
	var b strings.Builder
	b.WriteString(`<div class="saved-page"><p class="lens-lead">Your reading, kept privately. Save something while browsing, then come back to it or ask Micro about it.</p>`)
	if ref := r.URL.Query().Get("item"); ref != "" {
		item, e := store.Source(ref)
		if e != nil {
			app.NotFound(w, r, "Item not found")
			return
		}
		b.WriteString(`<h2>` + html.EscapeString(item.Title) + `</h2>` + app.SaveControl(r, ref))
	} else if id := r.URL.Query().Get("id"); id != "" {
		item, e := store.Get(sess.Account, id)
		if e != nil {
			app.NotFound(w, r, "Saved item not found")
			return
		}
		if app.WantsJSON(r) {
			app.RespondJSON(w, item)
			return
		}
		b.WriteString(`<p><a href="/saved">← Saved</a></p><h2>` + html.EscapeString(item.Title) + `</h2><p>` + html.EscapeString(item.Excerpt) + `</p>`)
		b.WriteString(`<div class="reading-actions"><a href="` + html.EscapeString(item.URL) + `" rel="noopener noreferrer">Original ↗</a><a href="/agent/micro?saved=` + url.QueryEscape(item.ID) + `">Ask Micro</a></div>`)
		b.WriteString(`<form method="POST" action="/saved">` + token(r) + hidden("action", "note") + hidden("id", item.ID) + `<label for="saved-note">Private note</label><textarea id="saved-note" name="note" rows="4" maxlength="4000">` + html.EscapeString(item.Note) + `</textarea><button>Save note</button></form>`)
		b.WriteString(`<form method="POST" action="/saved" class="reading-actions">` + token(r) + hidden("action", "delete") + hidden("id", item.ID) + `<button>Remove from Saved</button></form>`)
	} else {
		items, total, e := store.List(sess.Account, query, kind, offset, 20)
		if e != nil {
			app.Error(w, r, 500, "Could not load saved items")
			return
		}
		if app.WantsJSON(r) {
			app.RespondJSON(w, map[string]any{"items": items, "total": total})
			return
		}
		b.WriteString(`<form method="POST" action="/saved/search" class="saved-search">` + token(r) + `<input type="search" name="query" value="` + html.EscapeString(query) + `" placeholder="Search your saved items"><select name="kind" aria-label="Content type">`)
		for _, k := range []string{"", "article", "video", "post", "link"} {
			selected := ""
			if k == kind {
				selected = " selected"
			}
			label := k
			if k == "" {
				label = "All types"
			}
			b.WriteString(`<option value="` + k + `"` + selected + `>` + label + `</option>`)
		}
		b.WriteString(`</select><button>Search</button></form>`)
		if len(items) == 0 {
			if query != "" || kind != "" {
				b.WriteString(`<p>No saved items match this search. <a href="/saved">Show all saved items</a>.</p>`)
			} else {
				b.WriteString(`<p>Nothing saved here yet. Browse <a href="/news">News</a> or <a href="/video">Video</a>, or add a link below.</p>`)
			}
		}
		for _, i := range items {
			b.WriteString(`<article class="reading-row"><div class="reading-meta">` + html.EscapeString(i.Kind+" · "+i.Source+" · "+app.TimeAgo(i.Created)) + `</div><h3><a href="/saved?id=` + url.QueryEscape(i.ID) + `">` + html.EscapeString(i.Title) + `</a></h3><p>` + html.EscapeString(i.Note) + `</p><div class="reading-actions"><a href="` + html.EscapeString(i.URL) + `" rel="noopener noreferrer">Original ↗</a><a href="/agent/micro?saved=` + url.QueryEscape(i.ID) + `">Ask Micro</a></div></article>`)
		}
		b.WriteString(`<div class="reading-actions">`)
		for _, p := range []struct {
			label string
			off   int
		}{{"Previous", offset - 20}, {"Next", offset + 20}} {
			if p.off < 0 || p.off >= total {
				continue
			}
			b.WriteString(`<form method="POST" action="/saved/search">` + token(r) + hidden("query", query) + hidden("kind", kind) + hidden("offset", strconv.Itoa(p.off)) + `<button>` + p.label + `</button></form>`)
		}
		b.WriteString(`</div><details><summary>Add a link</summary><form method="POST" action="/saved" class="saved-add">` + token(r) + hidden("action", "add") + `<input name="url" type="url" required maxlength="4096" placeholder="https://…" aria-label="Link"><input name="title" maxlength="1000" placeholder="Title" aria-label="Title"><button>Save link</button></form></details>`)
	}
	b.WriteString(`</div>` + app.ReadingCSS + `<style>.saved-page{max-width:800px}.saved-search,.saved-add{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:20px}.saved-search input{flex:1;min-width:160px}.saved-page textarea{display:block;width:100%;box-sizing:border-box;margin:8px 0}.saved-add{margin-top:12px}</style>`)
	app.Respond(w, r, app.Response{Title: "Saved", Description: "Your private saved reading", HTML: b.String()})
}
func hidden(name, value string) string {
	return fmt.Sprintf(`<input type="hidden" name="%s" value="%s">`, name, html.EscapeString(value))
}
func token(r *http.Request) string { return app.CSRFField(auth.CSRFToken(r)) }
