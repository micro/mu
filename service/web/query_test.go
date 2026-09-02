package web

// Where the query comes from, and where it must never come from.
//
// It is read from the POST body, and the URL is not consulted at all. That is
// the rule this page is built around — see Handler — and every way of starting
// a search has to go through it or it is not a rule, it is a default that three
// places happen to follow.

import (
	"os"
	"strings"
	"testing"
)

// Nothing anywhere links a search into a URL.
//
// Three things did, at different times, and each was found separately: the
// topic chips, the recent-search chips, and the apps SDK. All three had the
// same two faults — they did not work, because the handler does not read the
// URL, and on the way to not working they wrote what somebody searched for into
// the browser history and the TLS terminator's access log.
func TestNothingLinksASearchIntoAURL(t *testing.T) {
	for _, path := range []string{"search.go", "../apps/static/sdk.js"} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			trimmed := strings.TrimSpace(line)
			// The explanation of why this is forbidden is allowed to name it.
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if strings.Contains(line, "/web?q=") {
				t.Errorf("%s puts a search in a URL:\n  %s\n"+
					"It will not work — the handler reads the body — and it leaks\n"+
					"the query into history and into the access log.", path, trimmed)
			}
		}
	}
}

// A recent search posts the form it is sitting next to.
func TestARecentSearchSubmitsTheForm(t *testing.T) {
	b, err := os.ReadFile("search.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if !strings.Contains(src, "form.submit(); return;") {
		t.Error("clicking a recent search does not submit the search form, so it\n" +
			"either does nothing or navigates with the query in the URL")
	}
	// And the form it submits has to be findable from the chip's own page.
	if !strings.Contains(src, `id="web-search"`) {
		t.Error(`the search form has no id, so nothing can find it to submit`)
	}
}
