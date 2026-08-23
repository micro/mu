package apps

// An app cannot act as the person looking at it.
//
// Apps are public, searchable, forkable and one click to open. A saved app used
// to be served as a full page on this origin with the viewer's session, and its
// SDK advertised raw get/post helpers "for any Mu endpoint" — so opening
// somebody's app could move your credits or read your mail, and CSRF was no
// obstacle because the token sits in a cookie JavaScript can read.
//
// These are the properties that stop that. Each one alone is bypassable, which
// is why all of them are checked.

import (
	"strings"
	"testing"

	"mu/internal/service"
)

// The app's own document must be an opaque origin, so its scripts have no
// cookies and no storage belonging to this site.
func TestTheAppDocumentIsServedIntoAnOpaqueOrigin(t *testing.T) {
	if !strings.Contains(sandboxCSP, "sandbox allow-scripts") {
		t.Fatal("app documents are no longer sandboxed")
	}
	if strings.Contains(sandboxCSP, "allow-same-origin") {
		t.Error("allow-same-origin puts the app back on this origin with the " +
			"viewer's cookies, which is the whole vulnerability")
	}
	// It cannot call out on its own either — the bridge is the only way, and
	// the bridge only does what the table permits.
	if !strings.Contains(sandboxCSP, "connect-src 'none'") {
		t.Error("the app can make its own network requests, so the op table is " +
			"advisory rather than a boundary")
	}
}

func TestTheFrameDoesNotGrantSameOrigin(t *testing.T) {
	page := sandboxPage("notes", "Notes")
	if !strings.Contains(page, `sandbox="allow-scripts`) {
		t.Fatal("the iframe is not sandboxed")
	}
	if strings.Contains(page, "allow-same-origin") {
		t.Error("the iframe grants same-origin")
	}
	if !strings.Contains(page, `src="/apps/notes?raw=1"`) {
		t.Error("the frame does not load the app's own document")
	}

	// No white flash between the two documents. Opening an app is two paints
	// and cannot be one — inlining the app would put untrusted HTML in this
	// origin — so what is left is making the gap the reader's own background
	// rather than a hard white, which in dark mode was a full-screen flash on
	// every app opened.
	if strings.Contains(page, "background:#fff") {
		t.Error("the frame flashes white while the app loads")
	}
	if !strings.Contains(page, "color-scheme:light dark") {
		t.Error("the frame does not follow the reader's colour scheme")
	}
}

// The shim inside the sandbox must not be able to fetch, and must not still
// offer the arbitrary-path helpers.
func TestTheShimCannotReachAnyEndpointItLikes(t *testing.T) {
	if strings.Contains(appShimJS, "fetch(") {
		t.Error("the in-sandbox shim calls fetch — it must ask the parent instead")
	}
	if strings.Contains(appShimJS, "document.cookie") {
		t.Error("the in-sandbox shim reads cookies")
	}
	// mu.get/mu.post are kept, refusing loudly, so an app written against them
	// says why it stopped rather than silently doing nothing.
	for _, want := range []string{"mu.get(path) is gone", "mu.post(path, body) is gone"} {
		if !strings.Contains(appShimJS, want) {
			t.Errorf("the shim no longer explains that %q was removed", want)
		}
	}
}

// The bridge is the boundary: it dispatches from a table of operations, never
// from a path the app supplies.
func TestTheBridgeDispatchesFromTheTableNotFromAPath(t *testing.T) {
	bridge := appBridgeJS("notes")

	if !strings.Contains(bridge, "e.source!==frame.contentWindow") {
		t.Error("the bridge answers messages from any window, so any page that " +
			"frames this one can drive it")
	}
	if !strings.Contains(bridge, "var spec=OPS[op]") {
		t.Error("the bridge no longer looks the operation up in the table")
	}
	if !strings.Contains(bridge, "not allowed to do") {
		t.Error("an unknown operation is not refused")
	}
	// A path taken straight from the message would restore the hole in a
	// different shape.
	for _, forbidden := range []string{"args.path", "m.path", "args.url"} {
		if strings.Contains(bridge, forbidden) {
			t.Errorf("the bridge takes %s from the app — the app is naming a URL again", forbidden)
		}
	}
}

// What an app may do is a list, and the dangerous things are not on it.
func TestTheOpTableGrantsNothingItShouldNot(t *testing.T) {
	for _, op := range bridgeOps {
		if op.Method != "GET" && op.Method != "POST" {
			t.Errorf("%s %s: only GET and POST are dispatched", op.Method, op.Path)
		}
		for _, off := range []string{"/wallet", "/account", "/admin", "/token", "/agents", "/mail", "/session/"} {
			if strings.HasPrefix(op.Path, off) {
				t.Errorf("an app can reach %s — money, identity and credentials are "+
					"not app capabilities", op.Path)
			}
		}
	}
	// The server-side proxy binds the caller itself and is where anything
	// per-user belongs.
	for _, want := range []string{"db", "store", "ai", "fetch", "service"} {
		if !sdkProxyOps[want] {
			t.Errorf("%s is no longer proxied server-side", want)
		}
	}
}

// Everything the shim asks for has to exist on the other side, or an app breaks
// at runtime with "not allowed to do" for something it is supposed to be able
// to do.
func TestEveryOperationTheShimUsesIsGranted(t *testing.T) {
	for _, op := range []string{
		"weather", "news", "markets", "video", "social", "search", "chat",
		"blog.list", "blog.read", "blog.create",
		"places.search", "places.nearby", "apps.list", "apps.read",
		"agent", "user",
	} {
		if _, ok := bridgeOps[op]; !ok {
			t.Errorf("the shim calls %q and the table does not grant it", op)
		}
		if !strings.Contains(appShimJS, "'"+op+"'") {
			t.Errorf("%q is granted but the shim never asks for it", op)
		}
	}
}

// An app reaches the instance's data, not the viewer's.
//
// The bridge and the CSP stop an app fetching what it likes, and mu.service
// went straight past both: it dispatches through the live registry with the
// viewer bound as the caller server-side, and allowed every registered service
// except "apps". So an app could call wallet.Charge and spend the person's
// credits, mail.Inbox and read their mail, docs and read their own documents — as
// them, on one click, from a public list. Sandboxing the code and leaving this
// open would have been theatre.
func TestAnAppCannotReachTheViewersOwnServices(t *testing.T) {
	for _, personal := range []string{
		"wallet", "mail", "contacts", "files", "events", "tasks", "docs", "images", "notes",
	} {
		if sdkServiceAllowed(personal) {
			t.Errorf("an app can call the %s service as whoever opened it", personal)
		}
	}
}

// And still reaches everything that is the instance's to give.
func TestAnAppStillReachesThePublicServices(t *testing.T) {
	registered := map[string]bool{}
	for _, s := range service.Services() {
		registered[s] = true
	}
	checked := 0
	for _, public := range []string{
		"news", "markets", "weather", "blog", "places", "prayer",
		"social", "stream", "video", "web", "chat",
	} {
		if !registered[public] {
			continue // not linked into this test binary
		}
		checked++
		if !sdkServiceAllowed(public) {
			t.Errorf("an app can no longer read %s, which is this instance's "+
				"data and not anybody's in particular", public)
		}
	}
	if checked == 0 {
		t.Skip("no public services registered in this binary")
	}
}

// App management is not an app capability, whatever else changes.
func TestAnAppCannotDriveApps(t *testing.T) {
	if sdkServiceAllowed("apps") {
		t.Error("an app can rewrite or run other apps")
	}
}
