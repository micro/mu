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
	"sort"
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
}

// What this server has read stays openable by a person, without asking the
// agent for permission to look.
//
// That was asserted on the footer, which is one route and turned out to be the
// wrong one to pin: the footer is where a site keeps the addresses a stranger
// has a question about, and a corpus is not one of those. The archive is on the
// row of doors under the box on the front page, which is where somebody goes
// looking, and /about links it for the pages that have no doors row.
//
// The property is the reachability, so that is what this checks.
func TestTheArchiveIsReachableWithoutTheAgent(t *testing.T) {
	src, err := os.ReadFile(repoRoot(t) + "/home/about.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `href="/archive"`) {
		t.Error("nothing on /about reaches the archive, and it is out of the\n" +
			"footer — so everything this server has read is behind the agent")
	}
}

// Alphabetical, and carrying the two questions a stranger actually has.
//
// Any other order is an argument about which destination matters most, and a
// footer is the one place on a page that is not making one — it is where a site
// keeps its addresses. A ranked row also has to be re-ranked every time
// something is added, which nobody ever does.
func TestTheFooterIsAlphabeticalAndSaysWhatThisIs(t *testing.T) {
	f := app.FooterLinks()
	for _, want := range []string{`href="/about"`, `href="/contact"`} {
		if !strings.Contains(f, want) {
			t.Errorf("the footer does not carry %s — the two questions somebody "+
				"with no account has are what this is and how to use it", want)
		}
	}
	var order []string
	for _, m := range footerHref.FindAllStringSubmatch(f, -1) {
		order = append(order, m[1])
	}
	if len(order) < 4 {
		t.Fatalf("found %d footer links — this scan is broken, not the code", len(order))
	}
	if !sort.StringsAreSorted(order) {
		t.Errorf("the footer is not alphabetical: %v", order)
	}
}
