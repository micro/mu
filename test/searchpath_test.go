package test

// /search answers.
//
// It 404ed. The apps SDK called it — mu.search() was get('/search?q=…') — so
// every app that searched got nothing back and said nothing about why, and it
// is also the address a person types on a product whose front door is a search
// box. Reported by opening /search?q=bitchat and getting a 404 page.
//
// The SDK names /web now, which fixes apps from here. It does not fix an
// address that has already been handed out: a URL in somebody's history, an app
// published against the old SDK, a link in a note. So the path answers too.

import (
	"os"
	"strings"
	"testing"
)

func TestSearchIsNotAFourOhFour(t *testing.T) {
	b, err := os.ReadFile(repoRoot(t) + "/internal/server/routes.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	if !strings.Contains(src, `http.HandleFunc("/search"`) {
		t.Fatal("/search is not registered, so it 404s")
	}
	// And it carries the query, or the redirect loses what was being searched
	// for — which is worse than the 404, because it looks like it worked.
	i := strings.Index(src, `http.HandleFunc("/search"`)
	body := src[i:]
	if j := strings.Index(body, "\n\thttp.HandleFunc"); j > 0 {
		body = body[:j]
	}
	if !strings.Contains(body, "r.URL.RawQuery") {
		t.Error("/search redirects without the query, so the search term is dropped")
	}
	if !strings.Contains(body, `"/web"`) {
		t.Error("/search does not point at /web")
	}
}

// And nothing in the product should be generating the old path any more.
func TestNothingLinksToTheOldSearchPath(t *testing.T) {
	root := repoRoot(t)
	for _, f := range []string{
		"/service/apps/static/sdk.js",
		"/service/web/search.go",
		"/service/video/video.go",
	} {
		b, err := os.ReadFile(root + f)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{`'/search?q='`, `"/search?q="`} {
			if strings.Contains(string(b), bad) {
				t.Errorf("%s still builds %s", f, bad)
			}
		}
	}
}
