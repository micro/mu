package app

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHeaderLoginPreservesDestination(t *testing.T) {
	for _, path := range []string{"/blog", "/blog/example-post", "/blog?view=archive&page=2", "/pricing"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		Respond(w, r, Response{Title: "Page", HTML: "<p>Content</p>"})
		want := `href="/login?redirect=` + url.QueryEscape(path) + `"`
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("%s: missing return destination", path)
		}
	}
}
