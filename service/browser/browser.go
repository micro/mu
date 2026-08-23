// Package browser is a real browser, for the pages a fetch cannot read.
//
// service/web fetches a URL and runs it through a readability pass, which is
// the right and cheap answer for a document that arrives as HTML. Most of the
// web has not worked that way for a decade: the response is a shell, the
// content arrives from JavaScript afterwards, and web_fetch comes back with a
// nav bar and an empty div. An agent asked to read such a page reports that it
// is empty, which is worse than failing, because it sounds like an answer.
//
// # Why it is its own service and not web.Render
//
// Because a browser is a domain, not a technique. Reading a rendered page is
// the first thing anybody wants from one, and it is not the last: a picture of
// a page, a PDF of it, and eventually filling in a form and pressing a button.
// browser.Click reads correctly and web.Click does not, which is the test for
// whether a noun is really the noun.
//
// The other half of the argument is money. web_fetch is free — it costs this
// instance an HTTP request. This costs a Chromium process, a few hundred
// megabytes and seconds of CPU per call, and it is priced accordingly. Two
// operations with costs three orders of magnitude apart do not belong behind
// one name.
//
// # The dependency, said plainly
//
// This needs Chromium. That is a real cost against "single Go binary,
// self-hostable" and it is not paid over: an instance with no browser
// configured says so and serves nothing, the same way service/maps behaves
// without an Ordnance Survey key.
//
// What keeps the binary single is that the browser does not have to be here.
// BROWSER_URL points at a DevTools endpoint anywhere — a container on the same
// host, a box on the network, one of the hosted ones — and chromedp talks to it
// over the wire. CHROME_PATH is for an operator who does want it local. Neither
// is required to build or run Mu; both are required to use this service, which
// is the honest shape for a capability that needs a program we did not write.
//
// # What an agent is pointed at
//
// A URL an agent chose, having read text a stranger wrote. So every request
// goes through internal/hosts first — this instance's own admin surface is at
// 127.0.0.1 and the cloud metadata endpoint hands out credentials to anything
// that asks it. Both are ordinary URLs to a browser.
package browser

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/hosts"
	"mu/internal/quota"
	"mu/internal/service"
	"mu/internal/settings"
)

// Server is the service handler.
type Server struct{}

// wait is how long one page is given, end to end.
//
// A browser will sit on a page that never settles for as long as you let it,
// and every second is a Chromium holding memory. Thirty is generous for a page
// worth reading and short enough that a hung one does not become the instance's
// problem.
const wait = 30 * time.Second

// maxText bounds what comes back from a page. A model reads this, and the
// budget for a web page in a context window is a few thousand words, not the
// whole of a documentation site rendered into one string.
const maxText = 40000

// Configured reports whether this instance has a browser to drive.
//
// Named for the question rather than the setting, because there are two ways to
// answer it and a caller should not have to know which one an operator chose.
func Configured() bool { return endpoint() != "" || binary() != "" }

// endpoint is a DevTools address to connect to, if one is set.
func endpoint() string { return strings.TrimSpace(settings.Get("BROWSER_URL")) }

// binary is a local Chromium to start, if one is set.
func binary() string { return strings.TrimSpace(settings.Get("CHROME_PATH")) }

// ── Read ────────────────────────────────────────────────────────

type ReadRequest struct {
	URL string `json:"url" required:"true" description:"The page to open"`
	// Wait is a CSS selector to wait for before reading, for a page whose
	// content arrives after the first paint and after the load event.
	Wait string `json:"wait" description:"Optional CSS selector to wait for before reading, for content that arrives late"`
}

type ReadResponse struct {
	Title string `json:"title" description:"The page's title"`
	Text  string `json:"text" description:"The page's visible text, after its JavaScript has run"`
	URL   string `json:"url" description:"Where the browser ended up, which is not the URL asked for when the page redirected"`
}

// Read opens a page in a real browser and returns what it says once its
// JavaScript has run.
// @example {"url": "https://example.com"}
func (Server) Read(ctx context.Context, req *ReadRequest, rsp *ReadResponse) error {
	target, err := checked(req.URL)
	if err != nil {
		return err
	}
	page, err := read(ctx, target, strings.TrimSpace(req.Wait))
	if err != nil {
		return err
	}
	rsp.Title, rsp.Text, rsp.URL = page.Title, page.Text, page.URL
	return nil
}

// ── Shot ────────────────────────────────────────────────────────

type ShotRequest struct {
	URL  string `json:"url" required:"true" description:"The page to photograph"`
	Full bool   `json:"full" description:"Capture the whole page rather than one screenful"`
	Wait string `json:"wait" description:"Optional CSS selector to wait for before the shot"`
}

type ShotResponse struct {
	URL   string `json:"url" description:"Where the picture is, on this instance"`
	Title string `json:"title" description:"The page's title"`
}

// Shot photographs a page and returns a URL for the picture.
//
// A URL rather than the bytes, for the same reason maps_tile answers with one:
// a PNG in a JSON field is a base64 blob no model can look at and no page can
// use, and everything that can show a picture takes a src.
// @example {"url": "https://example.com", "full": true}
func (Server) Shot(ctx context.Context, req *ShotRequest, rsp *ShotResponse) error {
	target, err := checked(req.URL)
	if err != nil {
		return err
	}
	shot, err := capture(ctx, target, strings.TrimSpace(req.Wait), req.Full)
	if err != nil {
		return err
	}
	rsp.URL, rsp.Title = shot.URL, shot.Title
	return nil
}

// checked is the URL, or why this instance will not open it.
func checked(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("a url is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	if err := hosts.FetchableString(raw); err != nil {
		return "", err
	}
	if !Configured() {
		return "", fmt.Errorf("this instance has no browser configured, so it cannot open pages: " +
			"an admin sets BROWSER_URL to a DevTools endpoint, or CHROME_PATH to a local Chromium")
	}
	return raw, nil
}

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("browser", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "browser",
	Handler:     new(Server),
	Description: "A real browser: read a page after its JavaScript has run, or photograph it",
	Page:        "/browser",
	Icon:        "browser.svg",
	Endpoints: map[string]service.Endpoint{
		"Read": {
			Cost: quota.OpBrowserRead,
			Doc: "Open a page in a real browser and read it after its JavaScript has run. " +
				"Use it when web_fetch comes back empty or with only a nav bar, which is what " +
				"a page that builds itself in the browser looks like to a plain fetch. " +
				"web_fetch is free and is the right first try; this costs, because it runs a browser",
		},
		"Shot": {
			Cost: quota.OpBrowserShot,
			Doc: "Photograph a page and get back a URL for the picture. What it looks like, " +
				"rather than what it says — a chart, a layout, a page whose content is an image. " +
				"Ask for full to capture past the first screenful",
		},
	},
}
