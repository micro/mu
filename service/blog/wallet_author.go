package blog

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/flag"
)

var walletAuthorID = regexp.MustCompile(`^x402:0x[0-9a-f]{40}$`)

// WalletAuthorHandler exposes public authorship, never an account or its mail.
func WalletAuthorHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(strings.TrimPrefix(r.URL.Path, "/@"))
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		app.MethodNotAllowed(w, r)
		return
	}
	if !walletAuthorID.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	var visible []*Post
	for _, p := range PostsByAuthorID(id, "") {
		if !p.Private && !flag.IsHidden("post", p.ID) && !auth.IsBanned(id) {
			visible = append(visible, p)
		}
	}
	if len(visible) == 0 {
		http.NotFound(w, r)
		return
	}
	sort.Slice(visible, func(i, j int) bool { return visible[i].CreatedAt.After(visible[j].CreatedAt) })
	var b strings.Builder
	b.WriteString(`<div class="page-col page-stack"><h1>Wallet author</h1><p class="pre-wrap">` + html.EscapeString(id) + `</p><p class="text-muted">Posts attributed to this verified wallet. Wallet identification does not mean a request was charged. Community posts are separate from Micro’s editorial publication.</p><div class="collection-list">`)
	for _, p := range visible {
		b.WriteString(`<a class="collection-item" href="/blog/post?id=` + url.QueryEscape(p.ID) + `">` + html.EscapeString(p.Title) + `</a>`)
	}
	b.WriteString(`</div></div>`)
	app.Respond(w, r, app.Response{Title: "Wallet author", HTML: b.String()})
}
