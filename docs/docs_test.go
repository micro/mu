package docs

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestGuidesResolveDocumentLinks(t *testing.T) {
	links := regexp.MustCompile(`href="([^"]+\.md(?:#[^"]*)?)"`)
	for _, p := range pages {
		t.Run(p.Path, func(t *testing.T) {
			w := httptest.NewRecorder()
			serve(w, httptest.NewRequest("GET", p.Path, nil), p)
			if w.Code != 200 {
				t.Fatalf("status %d", w.Code)
			}
			body := w.Body.String()
			if unresolved := links.FindAllString(body, -1); len(unresolved) > 0 {
				t.Errorf("unresolved document links: %v", unresolved)
			}
			if strings.Contains(body, "<h1>Install Micro</h1>") {
				t.Error("duplicate document title")
			}
		})
	}
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/help/missing", nil))
	if w.Code != 404 {
		t.Fatalf("unknown guide status %d", w.Code)
	}
}
