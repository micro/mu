package apps

// Putting an app on somebody else's page.
//
// # What this replaces
//
// There was an endpoint called Run. It took a snippet of JavaScript, kept it in
// a map in memory for an hour, and handed back an id — while its own
// documentation promised a URL. Nothing ran: the code executed in whoever
// opened the link's browser, and the caller never saw the result. It also
// carried a second copy of the sandbox — its own CSP literal, its own window.mu
// — and the two had drifted, so a snippet there could make network requests
// that a saved app could not.
//
// It is gone, and none of it is missed. These are static pages; the browser
// runs them, so "run" was never a thing this service did. What somebody
// actually wants after building one is to put it somewhere, and the app world
// has two verbs for that: create and embed.
//
// The hour was the strangest part. An artefact that deletes itself is either a
// draft, in which case it should be an app you have not published, or it is a
// mistake. Create already covers the first.
//
// # Why it is only an iframe
//
// Because that is the whole of it. An app is a document served at a URL, in an
// opaque origin, with a content policy that already assumes it is untrusted —
// see handleApp and sandboxCSP. Framing one is not a feature to build; it is
// the thing that was already true and had one header stopping it.
//
// What is here is therefore the two facts a person cannot work out for
// themselves: the absolute URL, and which apps it will not work for.

import (
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// EmbedHTML is the markup that puts an app on another page.
//
// The raw document rather than the framed one. /apps/<slug> is our page — the
// chrome, the title, the bridge — and nesting that inside somebody else's site
// gives them a frame of our furniture. ?raw=1 is the app itself.
func EmbedHTML(base, slug, name string) string {
	src := strings.TrimRight(base, "/") + "/apps/" + slug + "?raw=1"
	return `<iframe src="` + html.EscapeString(src) + `" ` +
		`title="` + html.EscapeString(name) + `" ` +
		`width="100%" height="480" style="border:0" ` +
		`sandbox="allow-scripts allow-forms allow-popups allow-modals" ` +
		`loading="lazy"></iframe>`
}

// bridged reports whether an app talks to this instance.
//
// It matters here and nowhere else. Inside Mu the page around the frame runs
// appBridgeJS, which answers the shim's postMessage; on somebody else's site
// there is nothing listening, so every mu.* call sits for sixty seconds and
// then rejects with "timed out". That is a worse failure than a refusal,
// because it looks like the app is slow rather than in the wrong place.
//
// So an app that uses the bridge still embeds — a checklist that only writes to
// mu.store is perfectly good on your own page for a reader who does not need it
// saved — but the answer says what will not work rather than letting somebody
// find out from a stranger.
func bridged(html string) bool { return bridgeCallRe.MatchString(html) }

// bridgeCallRe matches a call into window.mu: the word, a dot, and a letter.
// Loose on purpose — this decides whether to print a warning, so a false
// positive costs a sentence.
var bridgeCallRe = regexp.MustCompile(`(?i)\b(?:window\.)?mu\.[a-z]`)

// handleEmbed is the page at /apps/<slug>/embed: the tag for one app.
//
// It was a page of its own listing every app you own, reached from a link
// beside "+ New app". That is one more place to go and one more list to read,
// when the row you are looking at already names the app you mean — so it took
// the slot Launch had, which was the app's own title link said twice.
//
// Anybody's app, not only your own. The tag points at a public URL that anybody
// can already open, and the earlier restriction was about a page offering a
// directory of other people's work to paste; a button on the row of the thing
// you are reading about is not that.
func handleEmbed(w http.ResponseWriter, r *http.Request, slug string) {
	a := GetApp(slug)
	if a == nil {
		app.NotFound(w, r, "no app called "+slug)
		return
	}
	_, acc := auth.TrySession(r)
	if !a.Public && (acc == nil || acc.ID != a.AuthorID) {
		app.NotFound(w, r, "no app called "+slug)
		return
	}

	var b strings.Builder
	b.WriteString(`<div class="embed-page">`)
	b.WriteString(app.Actions(app.TextLink("← Apps", "/apps"),
		app.TextLink("Open "+a.Name, "/apps/"+a.Slug)))
	b.WriteString(`<h2 class="embed-title">Embed ` + html.EscapeString(a.Name) + `</h2>`)
	b.WriteString(`<p class="embed-lead">An app is a page at a URL. Put this on ` +
		`any site and it runs there, sandboxed, the same way it runs here.</p>`)

	// Paid apps are not offered. ?raw=1 is what the tag points at and it is the
	// path that does not charge — see handleApp, where the count and the charge
	// are both skipped for the raw document. Handing out a tag that bypasses
	// the price is worse than not offering one.
	if a.Price > 0 {
		b.WriteString(`<p class="embed-no">This app charges ` + strconv.Itoa(a.Price) +
			` credits a use, and an embedded copy would not charge anything. ` +
			`Set its price to nothing to embed it.</p></div>`)
		app.Respond(w, r, app.Response{Title: "Embed", Description: "Put an app on another page",
			HTML: b.String()})
		return
	}

	b.WriteString(`<textarea class="embed-code" rows="4" readonly ` +
		`onclick="this.select()">` + html.EscapeString(EmbedHTML(app.BaseURL(r), a.Slug, a.Name)) +
		`</textarea>`)
	if bridged(a.RenderHTML()) {
		b.WriteString(`<p class="embed-warn">This one calls <code>mu.</code> — the store, ` +
			`a service, the agent. Those work on this site, where the page around the frame ` +
			`answers them, and nowhere else: elsewhere they wait and then fail. Everything ` +
			`that does not need Mu still works.</p>`)
	}
	b.WriteString(`</div>`)

	app.Respond(w, r, app.Response{Title: "Embed", Description: "Put an app on another page",
		HTML: b.String()})
}
