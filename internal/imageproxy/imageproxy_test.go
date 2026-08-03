package imageproxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-imageproxy-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// stub replaces the network for the duration of a test and counts fetches.
func stub(t *testing.T, body, ctype string, err error) *int {
	t.Helper()
	calls := 0
	var mu sync.Mutex
	prev := fetchRemote
	fetchRemote = func(_ context.Context, _ string) ([]byte, string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		if err != nil {
			return nil, "", err
		}
		return []byte(body), ctype, nil
	}
	t.Cleanup(func() {
		fetchRemote = prev
		reset()
	})
	return &calls
}

// reset clears everything a test may have cached.
func reset() {
	mu.Lock()
	index = map[string]entry{}
	mu.Unlock()
	failMu.Lock()
	failed = map[string]time.Time{}
	failMu.Unlock()
}

func serve(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", target, nil))
	return w
}

// The point of the whole thing: the reader's browser asks this instance, and
// this instance has the bytes.
func TestAnImageIsFetchedOnceAndServedFromHere(t *testing.T) {
	calls := stub(t, "PNGBYTES", "image/png", nil)

	local := URL("https://cdn.example/news/cover.jpg")
	if !strings.HasPrefix(local, Path+"?") {
		t.Fatalf("URL did not rewrite to this instance: %q", local)
	}

	w := serve(t, local)
	if w.Code != http.StatusOK {
		t.Fatalf("first request returned %d", w.Code)
	}
	if w.Body.String() != "PNGBYTES" {
		t.Errorf("wrong body: %q", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("content type %q", ct)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("a proxied image is served without nosniff")
	}

	// Second reader, same image: the publisher is not asked again. This is what
	// makes a page of five hundred cards one round of requests rather than five
	// hundred per visitor.
	if w := serve(t, local); w.Code != http.StatusOK || w.Body.String() != "PNGBYTES" {
		t.Fatalf("second request returned %d %q", w.Code, w.Body.String())
	}
	if *calls != 1 {
		t.Errorf("upstream was fetched %d times, want 1", *calls)
	}
}

// Not an open proxy. Without a signature from this instance, /img fetches
// nothing — otherwise anyone could use the server to make requests.
func TestOnlySignedURLsAreFetched(t *testing.T) {
	calls := stub(t, "PNGBYTES", "image/png", nil)

	for _, target := range []string{
		Path + "?u=" + "https%3A%2F%2Fcdn.example%2Fa.png",
		Path + "?u=" + "https%3A%2F%2Fcdn.example%2Fa.png" + "&s=deadbeef",
		Path,
	} {
		if w := serve(t, target); w.Code != http.StatusNotFound {
			t.Errorf("%s returned %d, want 404", target, w.Code)
		}
	}

	// And a signature is bound to its URL — it cannot be reused for another.
	signed := URL("https://cdn.example/a.png")
	swapped := strings.Replace(signed, "cdn.example%2Fa.png", "evil.example%2Fb.png", 1)
	if w := serve(t, swapped); w.Code != http.StatusNotFound {
		t.Errorf("a signature was accepted for a different URL: %d", w.Code)
	}

	if *calls != 0 {
		t.Errorf("upstream was fetched %d times for unsigned requests", *calls)
	}
}

// SVG is XML that can carry script, and this serves from Mu's own origin, so an
// SVG accepted here would run as Mu.
func TestSVGAndNonImagesAreRefused(t *testing.T) {
	for _, tc := range []struct{ body, ctype string }{
		{`<svg onload="alert(1)"></svg>`, "image/svg+xml"},
		{`<html>gotcha</html>`, "text/html"},
		{`{}`, "application/json"},
		{"PNGBYTES", ""},
	} {
		reset()
		stub(t, tc.body, tc.ctype, nil)
		w := serve(t, URL("https://cdn.example/x"))
		if w.Code == http.StatusOK {
			t.Errorf("%q was served from this origin", tc.ctype)
		}
		if strings.Contains(w.Body.String(), "gotcha") || strings.Contains(w.Body.String(), "alert(1)") {
			t.Errorf("%q was served from this origin: %q", tc.ctype, w.Body.String())
		}
	}
}

// A dead host on a page of five hundred cards must not become five hundred
// outbound requests per render.
func TestAFailedFetchIsNotRetriedImmediately(t *testing.T) {
	calls := stub(t, "", "", fmt.Errorf("connection refused"))

	local := URL("https://down.example/a.png")
	for i := 0; i < 5; i++ {
		w := serve(t, local)
		// Falls back to the original URL rather than showing nothing: some CDNs
		// refuse a datacentre IP and allow the reader's own.
		if w.Code != http.StatusFound {
			t.Fatalf("request %d returned %d, want a redirect to the original", i, w.Code)
		}
		if loc := w.Header().Get("Location"); loc != "https://down.example/a.png" {
			t.Fatalf("fell back to %q", loc)
		}
	}
	if *calls != 1 {
		t.Errorf("a dead host was fetched %d times, want 1", *calls)
	}
}

// Anything that is not an http(s) URL is left exactly as it was: a relative
// path is already ours, and a data: URI has nothing to fetch.
func TestLocalAndInlineURLsAreLeftAlone(t *testing.T) {
	for _, in := range []string{
		"/images/daily/2026-08-03",
		"data:image/png;base64,iVBOR",
		"",
	} {
		if got := URL(in); got != in {
			t.Errorf("URL(%q) = %q, want it unchanged", in, got)
		}
	}
}

// The sweeper is what stops the cache being the one thing on a self-hosted box
// that only ever grows.
func TestSweepDropsImagesNobodyIsAskingFor(t *testing.T) {
	stub(t, "PNGBYTES", "image/png", nil)

	fresh := URL("https://cdn.example/fresh.png")
	old := URL("https://cdn.example/old.png")
	serve(t, fresh)
	serve(t, old)

	mu.Lock()
	if len(index) != 2 {
		mu.Unlock()
		t.Fatalf("expected 2 cached images, have %d", len(index))
	}
	for h, e := range index {
		if strings.Contains(e.Key, hashOf("https://cdn.example/old.png")) {
			e.Fetched = time.Now().Add(-2 * keepFor)
			index[h] = e
		}
	}
	mu.Unlock()

	sweep()

	mu.RLock()
	defer mu.RUnlock()
	if len(index) != 1 {
		t.Fatalf("sweep left %d entries, want 1", len(index))
	}
	if _, ok := index[hashOf("https://cdn.example/fresh.png")]; !ok {
		t.Error("sweep dropped the image that is still current")
	}
}
