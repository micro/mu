package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRobotsPublicText(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		r := httptest.NewRequest(method, "/robots.txt", nil)
		w := httptest.NewRecorder()
		robotsHandler(w, r)
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
			t.Fatal(w.Code, w.Header())
		}
		if method == "GET" && !strings.Contains(w.Body.String(), "Disallow: /admin") {
			t.Fatal(w.Body.String())
		}
		if method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD body")
		}
	}
}
