package app

// The manifest, which is what decides whether this can be installed at all.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// It parses, and it says the three things an install depends on.
//
// id is the one that is easy to leave out and expensive to get wrong: without
// it a browser derives the app's identity from start_url, so changing where the
// app opens changes *which app it is*. An already-installed copy stops matching
// and reinstalling gets stuck — which is exactly what happened when start_url
// moved from "/" to "/home?from=app". Pinned to "/", start_url can move freely.
func TestTheManifestSaysWhichAppThisIs(t *testing.T) {
	raw, err := htmlFiles.ReadFile("html/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Name      string `json:"name"`
		ShortName string `json:"short_name"`
		ID        string `json:"id"`
		Scope     string `json:"scope"`
		StartURL  string `json:"start_url"`
		Display   string `json:"display"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("the manifest is not valid JSON, so nothing can install: %v", err)
	}
	if m.Name != "Micro" || m.ShortName != "Micro" {
		t.Errorf("installed app is %q/%q, want Micro/Micro", m.Name, m.ShortName)
	}
	if m.ID != "/" {
		t.Errorf(`id = %q, want "/" — without a stable id the app's identity is`+"\n"+
			"its start_url, so moving where it opens makes it a different app\n"+
			"and an installed copy can no longer be replaced", m.ID)
	}
	if m.StartURL == "" || !strings.HasPrefix(m.StartURL, m.Scope) {
		t.Errorf("start_url %q is not inside scope %q, so it will not launch", m.StartURL, m.Scope)
	}

	for _, tag := range []string{
		`<meta name="apple-mobile-web-app-title" content="Micro">`,
		`<meta name="application-name" content="Micro">`,
	} {
		if !strings.Contains(Template, tag) {
			t.Errorf("install metadata is missing %s", tag)
		}
	}
}

// And it is served as a manifest.
//
// .webmanifest is not in Go's mime table, so http.FileServer sniffed the bytes
// and answered text/plain — and with X-Content-Type-Options: nosniff on every
// response, a browser is then required to refuse it. It only showed on the
// uncompressed path: compressed() sets the type itself and every real browser
// sends Accept-Encoding: gzip, so the one request that got it wrong was the one
// nobody makes with a browser.
func TestTheManifestIsServedAsAManifest(t *testing.T) {
	h := Serve()
	for _, enc := range []string{"", "gzip"} {
		r := httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)
		if enc != "" {
			r.Header.Set("Accept-Encoding", enc)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if got := w.Header().Get("Content-Type"); got != "application/manifest+json" {
			t.Errorf("Accept-Encoding %q: Content-Type = %q, want application/manifest+json —\n"+
				"with nosniff set, anything else is a manifest the browser must refuse",
				enc, got)
		}
	}
	// And the types Go does know are still right, which is what asking
	// contentType about every path would have broken: it answers
	// application/octet-stream for anything unrecognised.
	for _, c := range []struct{ path, want string }{
		{"/icon-192.png", "image/png"},
		{"/mu.css", "text/css; charset=utf-8"},
	} {
		r := httptest.NewRequest(http.MethodGet, c.path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if got := w.Header().Get("Content-Type"); got != c.want {
			t.Errorf("%s served as %q, want %q", c.path, got, c.want)
		}
	}
}
