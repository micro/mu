package browser

// The page, and the pictures it takes.

import (
	"html"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/blob"
	"mu/internal/quota"
)

// ShotHandler serves a picture this instance took, at /browser/shot/<id>.png.
//
// Public, and safe to be. The id is half a SHA-256 of the URL and the shape, so
// it is only guessable by somebody who already knows which page was
// photographed — and every page photographed is one somebody could open
// themselves, because internal/hosts refused everything else. What it must not
// become is a general blob reader: the key is rebuilt from the path here rather
// than taken from it.
func ShotHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/browser/shot/")
	id = strings.TrimSuffix(id, ".png")
	if id == "" || !hexID(id) {
		app.NotFound(w, r, "no picture with that name")
		return
	}
	b, err := blob.Get(shotPrefix + id + ".png")
	if err != nil || len(b) == 0 {
		app.NotFound(w, r, "no picture with that name")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	// A picture of a page is a picture of that page: the key names the URL, so
	// the same key is always the same shot. It is not the page as it is now,
	// which is why this is not immutable for a year the way a map tile is.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(b) //nolint:errcheck
}

// hexID is whether a path segment is one of ours, rather than a way into the
// blob store. Thirty-two hex characters and nothing else — no slashes, no dots,
// no traversal to think about.
func hexID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// Handler is the page: type a URL, see what a browser sees.
//
// A working thing rather than a description of one, for the same reason /maps
// draws a map. The service's whole claim is that it reads pages a fetch cannot,
// and the only way to show that is to let somebody put in a page that a fetch
// cannot read.
func Handler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString(`<div class="browser-page">`)
	b.WriteString(`<p class="svc-lead">` + Spec.Description + `. Most of the web builds ` +
		`itself in the browser, so a plain fetch of it comes back with a nav bar and an ` +
		`empty box — this is what the page actually says once its scripts have run.</p>`)

	if !Configured() {
		b.WriteString(app.Problem("This instance has no browser configured, so it cannot " +
			"open pages. An admin sets BROWSER_URL to a DevTools endpoint — a Chromium " +
			"container, or a hosted one — or CHROME_PATH to a Chromium on this machine."))
		b.WriteString(`</div>`)
		app.Respond(w, r, app.Response{Title: "Browser", Description: Spec.Description, HTML: b.String()})
		return
	}

	target := strings.TrimSpace(r.URL.Query().Get("url"))
	b.WriteString(`<form class="browser-form" method="get" action="/browser">`)
	b.WriteString(`<input class="browser-url" type="text" name="url" placeholder="https://example.com" ` +
		`value="` + html.EscapeString(target) + `">`)
	b.WriteString(`<button type="submit">Read</button></form>`)

	if target != "" {
		b.WriteString(reading(r, target))
	} else {
		b.WriteString(app.NoteHTML(`Reading a page costs ` +
			credits(quota.OperationCost(quota.OpBrowserRead)) + ` and a picture of one costs ` +
			credits(quota.OperationCost(quota.OpBrowserShot)) + `, because both run a browser. ` +
			`<code>web_fetch</code> is free and is the right first try; this is for ` +
			`the pages it comes back empty on.`))
	}

	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: "Browser", Description: Spec.Description, HTML: b.String()})
}

// credits writes a price the way a person reads one.
func credits(n int) string {
	if n == 1 {
		return "1 credit"
	}
	return strconv.Itoa(n) + " credits"
}

// reading opens the page somebody asked for and shows it.
//
// Charged, because it is the same work the tool does and the page is not a way
// round the price. Signed in, for the same reason: a meter needs somebody to
// bill, and an anonymous button that starts a browser is a free Chromium for
// anybody who finds the URL.
func reading(r *http.Request, target string) string {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		return `<p class="browser-problem">` +
			app.TextLink("Sign in", "/login?redirect=/browser") +
			` to read a page here — running a browser costs, so it needs an account to bill.</p>`
	}

	checkedURL, err := checked(target)
	if err != nil {
		return `<p class="browser-problem">` + html.EscapeString(err.Error()) + `</p>`
	}
	// The same gate the tool goes through. The page is not a way round the
	// price: it does the work the tool does, so it costs what the tool costs.
	ok, _, cost, qerr := quota.CheckQuota(acc.ID, quota.OpBrowserRead)
	if qerr != nil {
		return `<p class="browser-problem">` + html.EscapeString(qerr.Error()) + `</p>`
	}
	if !ok {
		return quota.ExceededPage(cost)
	}

	page, err := read(r.Context(), checkedURL, "")
	if err != nil {
		return `<p class="browser-problem">` + html.EscapeString(err.Error()) + `</p>`
	}
	// Charged after it worked. A page that would not load is not a page anybody
	// should pay for, and the browser is the part that can fail.
	quota.Charge(acc.ID, quota.OpBrowserRead, map[string]interface{}{"url": checkedURL}) //nolint:errcheck

	var b strings.Builder
	b.WriteString(`<div class="browser-out">`)
	title := page.Title
	if strings.TrimSpace(title) == "" {
		title = page.URL
	}
	b.WriteString(`<h2 class="browser-title">` + html.EscapeString(title) + `</h2>`)
	if page.URL != "" && page.URL != checkedURL {
		b.WriteString(`<p class="browser-where">redirected to <code>` +
			html.EscapeString(page.URL) + `</code></p>`)
	}
	if strings.TrimSpace(page.Text) == "" {
		b.WriteString(`<p class="browser-problem">That page rendered nothing readable. ` +
			`It may need a moment longer, or a selector to wait for — the tool takes one.</p>`)
	} else {
		b.WriteString(`<pre class="browser-text">` + html.EscapeString(page.Text) + `</pre>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}
