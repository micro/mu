package app

// Pages revalidate; assets do not.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A page went out with no cache headers at all, which does not mean "do not
// cache" — it means the browser guesses, and an installed app guessing wrong
// holds a page for as long as it likes. That is how a fix ships and somebody on
// their home screen keeps the old screen: the assets are versioned and the page
// that names those versions was not, so a stale page went on asking for the
// stale stylesheet it was built against, and its inline script stayed whatever
// it was the day it was cached.
func TestAPageHasToAskBeforeItIsReused(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/anything", nil)
	w := httptest.NewRecorder()
	Respond(w, r, Response{Title: "T", Description: "D", HTML: "<p>hi</p>"})

	got := w.Header().Get("Cache-Control")
	if got == "" {
		t.Fatal("a page goes out with no Cache-Control, so how long it is held\n" +
			"is the browser's guess — which is how a deployed fix fails to reach\n" +
			"an installed app")
	}
	for _, want := range []string{"no-cache", "private"} {
		if !strings.Contains(got, want) {
			t.Errorf("Cache-Control = %q, missing %q", got, want)
		}
	}
	// no-store would be wrong: the page is still worth caching, it just has to
	// revalidate, so an unchanged one costs a 304 rather than a download.
	if strings.Contains(got, "no-store") {
		t.Errorf("Cache-Control = %q — no-store throws the page away entirely, "+
			"which spends a download on every navigation", got)
	}
}

// And the assets keep their long life, because their URLs carry a version and
// a versioned URL is safe to hold forever.
func TestAssetsAreStillCachedHard(t *testing.T) {
	h := Serve()
	for _, path := range []string{"/mu.css", "/icon-192.png"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "max-age") {
			t.Errorf("%s: Cache-Control = %q, want a long max-age — the URL "+
				"carries a version, so holding it costs nothing", path, got)
		}
	}
	// Except the service worker, which is installed rather than loaded: under
	// the HTTP cache an update check gets the cached bytes back, finds them
	// identical, and keeps the worker it has.
	r := httptest.NewRequest(http.MethodGet, "/mu.js", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("/mu.js: Cache-Control = %q, want no-cache", got)
	}
}
