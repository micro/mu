package test

// Every link in the footer opens for a stranger.
//
// The footer is rendered on the signed-out page and nowhere else — a signed-in
// account gets the sidebar, which TestEveryFooterLinkIsReachableSignedIn
// asserts. So its entire audience is people with no session, and a link that
// answers by redirecting to /login is not a destination, it is a bounce.
//
// This exists because Agent was put in the footer for an afternoon. /agent
// checks auth in its handler and redirects, so the one page the footer appears
// on was the one page the link could not work from — and it was simultaneously
// redundant for everybody who *could* use it, since Agents is in the sidebar.
// Caught by being asked "the /agent is not a public page though right", which
// is a question the tests should have been able to answer.
//
// The route table is the source of truth. An entry of true means the router
// requires a session; /agent is listed false and checks in the handler, so it
// is named here as well — the ledger is short and being explicit about the
// exception is better than a scan that quietly misses it.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"mu/internal/app"
)

// gatedInHandler are paths the route table calls public and the handler then
// refuses without a session. Adding one here is a decision, which is the point.
var gatedInHandler = []string{"/agent", "/agents"}

// routeGate matches `"/path": true,` in the route table.
var routeGate = regexp.MustCompile(`"(/[a-z0-9/-]*)":\s*(true|false)`)

func TestEveryFooterLinkOpensForAStranger(t *testing.T) {
	b, err := os.ReadFile(repoRoot(t) + "/internal/server/routes.go")
	if err != nil {
		t.Fatal(err)
	}

	gated := map[string]bool{}
	for _, m := range routeGate.FindAllStringSubmatch(string(b), -1) {
		if m[2] == "true" {
			gated[m[1]] = true
		}
	}
	for _, p := range gatedInHandler {
		gated[p] = true
	}
	if len(gated) < 5 {
		t.Fatalf("only %d gated paths found — this scan is broken, not the code", len(gated))
	}

	links := footerHref.FindAllStringSubmatch(app.FooterLinks(), -1)
	if len(links) < 4 {
		t.Fatalf("found %d footer links — this scan is broken, not the code", len(links))
	}
	for _, m := range links {
		href := m[1]
		if gated[href] {
			t.Errorf("%s is in the footer and needs a session. The footer is only "+
				"rendered signed out, so this link bounces every reader who sees it.", href)
		}
	}
	// And the one that is the point: what this server remembers is public, and
	// a stranger can open it without asking the agent for permission to look.
	if !strings.Contains(app.FooterLinks(), `href="/archive"`) {
		t.Error("the archive is not in the footer — everything the agent reads " +
			"has to be somewhere a person can read directly")
	}
}
