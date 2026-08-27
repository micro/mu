package docs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// One page, and it is a file that exists.
//
// There were nine, behind a categorised index. The index was the tell: a set of
// documents large enough to need navigating is a manual, and the product was
// meant to explain itself — the tools are at /tools, the protocol is a URL, and
// the price list is a page.
//
// Then three, then one. /help was a second page about pointing an agent at this
// instance, which is what /tools is for. /about was a second answer to what the
// landing already says, and the landing is the page somebody deciding whether to
// care actually reads. Each time, the page that went was the one nobody
// maintained.
func TestEveryPageServes(t *testing.T) {
	if len(pages) != 1 {
		t.Fatalf("%d pages — Install is the whole site's documentation", len(pages))
	}
	for i, p := range pages {
		if _, err := docsFS.ReadFile(p.Filename); err != nil {
			t.Errorf("%s: %v", p.Path, err)
			continue
		}
		w := httptest.NewRecorder()
		serve(w, httptest.NewRequest(http.MethodGet, p.Path, nil), pages[i])
		if w.Code != http.StatusOK {
			t.Errorf("%s: status %d", p.Path, w.Code)
		}
		if !strings.Contains(w.Body.String(), "docs-content") {
			t.Errorf("%s: rendered nothing", p.Path)
		}
	}
}

// A doc opens with its own title, because it is a file first. The page shell
// renders the title above the content, so served as a page the heading appears
// twice ("Install / Install").
func TestServedPageDoesNotRepeatItsTitle(t *testing.T) {
	for i, p := range pages {
		w := httptest.NewRecorder()
		serve(w, httptest.NewRequest(http.MethodGet, p.Path, nil), pages[i])
		body := w.Body.String()
		if n := strings.Count(body, "<h1"); n > 1 {
			t.Errorf("%s: %d h1s, the title is rendered twice", p.Path, n)
		}
	}
}

// Every address the old nine answered on still goes somewhere.
//
// Not necessarily to a doc. When /help went, everything pointing at it was
// repointed to /tools; when /about went, to the landing. Both are pages in the
// product rather than files in this package, and both are what those addresses
// were actually for — which is the whole reason the pages could go.
func TestOldAddressesLand(t *testing.T) {
	known := map[string]bool{"/tools": true, "/": true}
	for _, p := range pages {
		known[p.Path] = true
	}
	for from, to := range Redirects {
		if !known[to] {
			t.Errorf("%s redirects to %s, which is not a page", from, to)
		}
		// /docs/*, /help/*, and /about, which was a page here until the landing
		// was agreed to be the one answer to what this is. The prefix rule is to
		// stop unrelated redirects being dumped in a map named for the
		// documentation's own addresses.
		if from != "/about" && !strings.HasPrefix(from, "/docs") && !strings.HasPrefix(from, "/help") {
			t.Errorf("%s is not an address the documentation ever had", from)
		}
	}
	for _, want := range []string{"/docs/mcp", "/docs/installation", "/docs/about", "/about"} {
		if Redirects[want] == "" {
			t.Errorf("%s has nowhere to go", want)
		}
	}
}

func TestStripTitle(t *testing.T) {
	got := string(stripTitle([]byte("# Install\n\nRun your own.\n")))
	if want := "Run your own.\n"; got != want {
		t.Errorf("stripTitle = %q, want %q", got, want)
	}
	if got := string(stripTitle([]byte("No heading here.\n"))); got != "No heading here.\n" {
		t.Errorf("a document with no H1 should be left alone, got %q", got)
	}
}
