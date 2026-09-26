package blog

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
)

// Drafts are private posts belonging to the signed-in author. Never put them
// in the shared public-list cache, even for an administrator.
func draftsHandler(w http.ResponseWriter, r *http.Request) {
	_, acc := auth.TrySession(r)
	w.Header().Set("Cache-Control", "private, no-store")
	if acc == nil {
		app.RedirectToLogin(w, r)
		return
	}
	mutex.RLock()
	drafts := []Post{}
	for _, p := range posts {
		if p.Private && p.AuthorID == acc.ID {
			drafts = append(drafts, *p)
		}
	}
	mutex.RUnlock()
	pager := app.Paginate(r, len(drafts), postsPerPage)
	if app.WantsJSON(r) {
		app.RespondJSON(w, drafts[pager.From:pager.To])
		return
	}
	var b strings.Builder
	b.WriteString(`<div id="blog" class="editorial-page"><p class="text-muted">Your private posts. Open a draft to read or edit it; choose Public in the editor when it is ready.</p><nav class="section-actions" aria-label="Blog"><a href="/blog">Editorial</a><a href="/blog?view=community">Community</a><a href="/blog?view=archive">Archive</a><a href="/blog?view=drafts" aria-current="page">Drafts</a><a href="/blog?write=true">Write a post</a></nav><div id="posts-list">`)
	if len(drafts) == 0 {
		b.WriteString(`<p class="text-muted">No drafts yet. Choose Private (draft) when saving a post to keep it here.</p>`)
	}
	for _, p := range drafts[pager.From:pager.To] {
		title := strings.TrimSpace(p.Title)
		if title == "" {
			title = "Untitled"
		}
		at := p.UpdatedAt
		if at.IsZero() {
			at = p.CreatedAt
		}
		href := "/blog/post?id=" + url.QueryEscape(p.ID)
		fmt.Fprintf(&b, `<article class="editorial-entry"><div class="metadata-row"><span>Private draft</span><time datetime="%s">%s</time></div><h2><a href="%s">%s</a></h2><p>%s</p><a class="btn" href="%s&amp;edit=true">Edit</a></article>`, at.Format(time.RFC3339), at.Format("2 January 2006"), href, html.EscapeString(title), postExcerpt(p.Content), href)
	}
	b.WriteString(`</div>` + pager.Nav("/blog?view=drafts") + `</div>`)
	app.Respond(w, r, app.Response{Title: "Drafts", BodyClass: "reading-page editorial-reading", HTML: b.String()})
}
